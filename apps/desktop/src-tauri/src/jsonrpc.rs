//! NDJSON JSONRPC 2.0 client. One `Client` owns a child process's stdin/stdout
//! and serializes calls through a request/response map keyed by id.

use anyhow::{anyhow, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::process::{ChildStdin, ChildStdout};
use tokio::sync::{oneshot, Mutex};

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

pub struct Client {
    next_id: AtomicI64,
    stdin: Mutex<ChildStdin>,
    pending: Pending,
}

impl Client {
    /// Spawn the reader task and return a Client. Notifications (no id) are
    /// dropped in Plan 0; the AI streaming work in Plan 5 will replace this.
    pub fn new(stdin: ChildStdin, stdout: ChildStdout) -> Arc<Self> {
        let pending: Pending = Arc::new(Mutex::new(Default::default()));
        let client = Arc::new(Client {
            next_id: AtomicI64::new(1),
            stdin: Mutex::new(stdin),
            pending: pending.clone(),
        });
        let pending_for_reader = pending.clone();
        tokio::spawn(async move {
            let mut reader = BufReader::new(stdout).lines();
            while let Ok(Some(line)) = reader.next_line().await {
                let resp: Response = match serde_json::from_str(&line) {
                    Ok(r) => r,
                    Err(_) => continue, // drop malformed lines
                };
                // Notifications (no id, has method) are ignored in Plan 0.
                if resp.method.is_some() && resp.id.is_none() {
                    let _ = resp.params; // touch field
                    continue;
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
            stdin.write_all(payload.as_bytes()).await?;
            stdin.write_all(b"\n").await?;
            stdin.flush().await?;
        }

        match rx.await {
            Ok(Ok(v)) => Ok(v),
            Ok(Err(e)) => Err(anyhow!("rpc error {}: {}", e.code, e.message)),
            Err(_) => Err(anyhow!("rpc channel closed before reply")),
        }
    }
}
