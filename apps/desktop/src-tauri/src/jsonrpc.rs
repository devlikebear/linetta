//! NDJSON JSONRPC 2.0 client. One `Client` owns a child process's stdin/stdout
//! and serializes calls through a request/response map keyed by id.

use anyhow::{anyhow, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;
use std::time::Duration;
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::process::{ChildStdin, ChildStdout};
use tokio::sync::{oneshot, Mutex};

/// Callback invoked for every JSONRPC notification received on the read side.
/// Method is the JSONRPC method name (e.g. "ai.delta"); params is the raw JSON.
pub type NotificationHandler = std::sync::Arc<dyn Fn(String, serde_json::Value) + Send + Sync>;

#[derive(Serialize)]
struct Request<'a> {
    jsonrpc: &'a str,
    id: i64,
    method: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    params: Option<&'a Value>,
}

#[derive(Deserialize)]
struct Response {
    #[allow(dead_code)]
    jsonrpc: Option<String>,
    id: Option<Value>,
    #[serde(default)]
    result: Option<Value>,
    #[serde(default)]
    error: Option<RpcError>,
    #[serde(default)]
    method: Option<String>,
    #[serde(default)]
    params: Option<Value>,
}

#[derive(Deserialize, Debug)]
pub struct RpcError {
    pub code: i64,
    pub message: String,
}

type Pending = Arc<Mutex<std::collections::HashMap<i64, oneshot::Sender<Result<Value, RpcError>>>>>;

const DEFAULT_CALL_TIMEOUT: Duration = Duration::from_secs(30);
const READER_CLOSED_CODE: i64 = -32099;

pub struct Client {
    next_id: AtomicI64,
    stdin: Mutex<Box<dyn AsyncWrite + Unpin + Send>>,
    pending: Pending,
    call_timeout: Duration,
}

impl Client {
    /// Spawn the reader task and return a Client. `on_notification` is called for
    /// each incoming JSONRPC notification (no id, has method).
    pub fn new(
        stdin: ChildStdin,
        stdout: ChildStdout,
        on_notification: NotificationHandler,
    ) -> Arc<Self> {
        Self::new_with_io(stdin, stdout, on_notification, DEFAULT_CALL_TIMEOUT)
    }

    fn new_with_io<W, R>(
        stdin: W,
        stdout: R,
        on_notification: NotificationHandler,
        call_timeout: Duration,
    ) -> Arc<Self>
    where
        W: AsyncWrite + Unpin + Send + 'static,
        R: AsyncRead + Unpin + Send + 'static,
    {
        let pending: Pending = Arc::new(Mutex::new(Default::default()));
        let client = Arc::new(Client {
            next_id: AtomicI64::new(1),
            stdin: Mutex::new(Box::new(stdin)),
            pending: pending.clone(),
            call_timeout,
        });
        let pending_for_reader = pending.clone();
        tokio::spawn(async move {
            let mut reader = BufReader::new(stdout).lines();
            loop {
                let line = match reader.next_line().await {
                    Ok(Some(line)) => line,
                    Ok(None) => {
                        drain_pending(&pending_for_reader, "rpc reader closed").await;
                        return;
                    }
                    Err(e) => {
                        drain_pending(&pending_for_reader, &format!("rpc reader error: {e}")).await;
                        return;
                    }
                };
                let resp: Response = match serde_json::from_str(&line) {
                    Ok(r) => r,
                    Err(_) => continue, // drop malformed lines
                };
                // Notifications: forward to the handler.
                if let Some(method) = resp.method.clone() {
                    if resp.id.is_none() {
                        let params = resp.params.clone().unwrap_or(Value::Null);
                        (on_notification)(method, params);
                        continue;
                    }
                }
                let id = match resp.id.as_ref().and_then(|v| v.as_i64()) {
                    Some(n) => n,
                    None => continue,
                };
                let tx = pending_for_reader.lock().await.remove(&id);
                if let Some(tx) = tx {
                    let payload: Result<Value, RpcError> = if let Some(err) = resp.error {
                        Err(err)
                    } else {
                        Ok(resp.result.unwrap_or(Value::Null))
                    };
                    let _ = tx.send(payload);
                }
            }
        });
        client
    }

    pub async fn call(&self, method: &str, params: Option<Value>) -> Result<Value> {
        let id = self.next_id.fetch_add(1, Ordering::Relaxed);
        let (tx, rx) = oneshot::channel();
        self.pending.lock().await.insert(id, tx);

        let payload = serde_json::to_string(&Request {
            jsonrpc: "2.0",
            id,
            method,
            params: params.as_ref(),
        })?;
        {
            let mut stdin = self.stdin.lock().await;
            if let Err(e) = stdin.write_all(payload.as_bytes()).await {
                self.pending.lock().await.remove(&id);
                return Err(e.into());
            }
            if let Err(e) = stdin.write_all(b"\n").await {
                self.pending.lock().await.remove(&id);
                return Err(e.into());
            }
            if let Err(e) = stdin.flush().await {
                self.pending.lock().await.remove(&id);
                return Err(e.into());
            }
        }

        match tokio::time::timeout(self.call_timeout, rx).await {
            Ok(Ok(Ok(v))) => Ok(v),
            Ok(Ok(Err(e))) => Err(anyhow!("rpc error {}: {}", e.code, e.message)),
            Ok(Err(_)) => Err(anyhow!("rpc channel closed before reply")),
            Err(_) => {
                self.pending.lock().await.remove(&id);
                Err(anyhow!(
                    "rpc timeout after {}ms",
                    self.call_timeout.as_millis()
                ))
            }
        }
    }
}

async fn drain_pending(pending: &Pending, message: &str) {
    let drained = std::mem::take(&mut *pending.lock().await);
    for (_, tx) in drained {
        let _ = tx.send(Err(RpcError {
            code: READER_CLOSED_CODE,
            message: message.to_string(),
        }));
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use std::sync::Mutex as StdMutex;
    use tokio::io::duplex;

    fn noop_handler() -> NotificationHandler {
        Arc::new(|_, _| {})
    }

    #[tokio::test]
    async fn routes_response_to_matching_call() {
        let (client_stdin, engine_read) = duplex(1024);
        let (mut engine_write, client_stdout) = duplex(1024);
        let client = Client::new_with_io(
            client_stdin,
            client_stdout,
            noop_handler(),
            Duration::from_secs(1),
        );

        let server = tokio::spawn(async move {
            let mut reader = BufReader::new(engine_read).lines();
            let line = reader.next_line().await.unwrap().unwrap();
            let req: Value = serde_json::from_str(&line).unwrap();
            assert_eq!(req["method"], "ping");
            let body = json!({"jsonrpc":"2.0","id":req["id"],"result":"pong"}).to_string();
            engine_write.write_all(body.as_bytes()).await.unwrap();
            engine_write.write_all(b"\n").await.unwrap();
        });

        let got = client.call("ping", None).await.unwrap();
        server.await.unwrap();
        assert_eq!(got, json!("pong"));
    }

    #[tokio::test]
    async fn forwards_notifications_without_pending_response() {
        let (client_stdin, _engine_read) = duplex(1024);
        let (mut engine_write, client_stdout) = duplex(1024);
        let seen: Arc<StdMutex<Vec<(String, Value)>>> = Arc::new(StdMutex::new(Vec::new()));
        let seen_for_handler = seen.clone();
        let _client = Client::new_with_io(
            client_stdin,
            client_stdout,
            Arc::new(move |method, params| {
                seen_for_handler.lock().unwrap().push((method, params));
            }),
            Duration::from_secs(1),
        );

        let body = json!({"jsonrpc":"2.0","method":"ai.delta","params":{"text":"hi"}}).to_string();
        engine_write.write_all(body.as_bytes()).await.unwrap();
        engine_write.write_all(b"\n").await.unwrap();
        tokio::time::sleep(Duration::from_millis(20)).await;

        let seen = seen.lock().unwrap();
        assert_eq!(seen.len(), 1);
        assert_eq!(seen[0].0, "ai.delta");
        assert_eq!(seen[0].1, json!({"text":"hi"}));
    }

    #[tokio::test]
    async fn times_out_pending_calls() {
        let (client_stdin, _engine_read) = duplex(1024);
        let (_engine_write, client_stdout) = duplex(1024);
        let client = Client::new_with_io(
            client_stdin,
            client_stdout,
            noop_handler(),
            Duration::from_millis(20),
        );

        let err = client.call("slow", None).await.unwrap_err().to_string();
        assert!(err.contains("rpc timeout"), "{err}");
    }

    #[tokio::test]
    async fn reader_eof_fails_pending_calls() {
        let (client_stdin, engine_read) = duplex(1024);
        let (engine_write, client_stdout) = duplex(1024);
        let client = Client::new_with_io(
            client_stdin,
            client_stdout,
            noop_handler(),
            Duration::from_secs(1),
        );

        let call = tokio::spawn({
            let client = client.clone();
            async move { client.call("will-close", None).await }
        });

        let mut reader = BufReader::new(engine_read).lines();
        let _ = reader.next_line().await.unwrap().unwrap();
        drop(engine_write);

        let err = call.await.unwrap().unwrap_err().to_string();
        assert!(err.contains("rpc reader closed"), "{err}");
    }
}
