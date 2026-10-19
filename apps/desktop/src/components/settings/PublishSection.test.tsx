import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  connectStart: vi.fn(),
  connectStatus: vi.fn(),
  disconnect: vi.fn(),
  openExternalUrl: vi.fn(),
}));

vi.mock("../../lib/rpc", () => ({
  publishApi: {
    connectStart: rpc.connectStart,
    connectStatus: rpc.connectStatus,
    disconnect: rpc.disconnect,
  },
  openExternalUrl: rpc.openExternalUrl,
}));

vi.mock("../../lib/i18n", () => ({
  // The keys are the contract under test, not the prose, so echo them back.
  useI18n: () => ({
    t: (key: string, vars?: Record<string, string>) =>
      vars ? `${key}:${Object.values(vars).join(",")}` : key,
  }),
}));

import { PublishSection } from "./PublishSection";

const SITE = "https://works.marvin-42.com";
const DISCONNECTED = { state: "disconnected", site_url: SITE };
const WAITING = {
  state: "waiting",
  site_url: SITE,
  confirm_code: "NWCQ-BKWX",
  verify_url: `${SITE}/connect?pairing=p1`,
};
const CONNECTED = { state: "connected", site_url: SITE, pen_name: "마곰", handle: "devlikebear" };

describe("PublishSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rpc.connectStatus.mockResolvedValue(DISCONNECTED);
    rpc.openExternalUrl.mockResolvedValue(undefined);
    rpc.disconnect.mockResolvedValue({ ok: true });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders nothing on a build that cannot publish, and asks nothing", () => {
    const { container } = render(<PublishSection available={false} />);
    expect(container).toBeEmptyDOMElement();
    expect(rpc.connectStatus).not.toHaveBeenCalled();
  });

  it("disconnected: says what is sent and will not connect until the writer agrees", async () => {
    render(<PublishSection available />);
    const connect = await screen.findByTestId("publish-connect");
    expect(screen.getByTestId("publish-disclosure")).toHaveTextContent("settings.publish.disclosure");
    expect(connect).toBeDisabled();
    expect(rpc.connectStart).not.toHaveBeenCalled();

    rpc.connectStart.mockResolvedValue(WAITING);
    await userEvent.click(screen.getByTestId("publish-consent"));
    expect(connect).toBeEnabled();
    await userEvent.click(connect);
    await waitFor(() => expect(rpc.connectStart).toHaveBeenCalledWith(true));
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(WAITING.verify_url);
  });

  it("links to the site without connecting or sending anything", async () => {
    render(<PublishSection available />);
    await userEvent.click(await screen.findByTestId("publish-browse"));
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(SITE);
    expect(rpc.connectStart).not.toHaveBeenCalled();
  });

  it("waiting: shows the confirm code, polls every 1.5s, and lands on connected", async () => {
    rpc.connectStatus.mockResolvedValueOnce(WAITING);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    render(<PublishSection available />);
    expect(await screen.findByTestId("publish-confirm-code")).toHaveTextContent("NWCQ-BKWX");

    rpc.connectStatus.mockResolvedValueOnce(WAITING).mockResolvedValue(CONNECTED);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(screen.getByTestId("publish-waiting")).toBeInTheDocument();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(await screen.findByTestId("publish-connected")).toHaveTextContent("마곰");

    // Connected ends the poll.
    const calls = rpc.connectStatus.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4500);
    });
    expect(rpc.connectStatus.mock.calls.length).toBe(calls);
  });

  it("waiting: cancel abandons the attempt and a late poll answer is ignored", async () => {
    rpc.connectStatus.mockResolvedValueOnce(WAITING);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    render(<PublishSection available />);
    await screen.findByTestId("publish-waiting");

    rpc.connectStatus.mockResolvedValue(DISCONNECTED);
    await userEvent.click(screen.getByTestId("publish-cancel"));
    await waitFor(() => expect(rpc.disconnect).toHaveBeenCalled());
    expect(await screen.findByTestId("publish-connect")).toBeInTheDocument();

    rpc.connectStatus.mockResolvedValue(CONNECTED);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(screen.queryByTestId("publish-connected")).not.toBeInTheDocument();
  });

  it("connected: shows the pen name, links to the site, and disconnects", async () => {
    rpc.connectStatus.mockResolvedValue(CONNECTED);
    render(<PublishSection available />);
    expect(await screen.findByTestId("publish-connected")).toHaveTextContent(
      "settings.publish.connected:마곰,devlikebear",
    );
    await userEvent.click(screen.getByTestId("publish-my-works"));
    expect(rpc.openExternalUrl).toHaveBeenCalledWith(`${SITE}/me`);

    rpc.connectStatus.mockResolvedValue(DISCONNECTED);
    await userEvent.click(screen.getByTestId("publish-disconnect"));
    await waitFor(() => expect(rpc.disconnect).toHaveBeenCalled());
    expect(await screen.findByTestId("publish-connect")).toBeDisabled();
  });

  it("failed: tells denied, expired, network and revoked apart, with a retry", async () => {
    for (const reason of ["denied", "expired", "network", "revoked"]) {
      rpc.connectStatus.mockResolvedValue({ state: "failed", site_url: SITE, failure_reason: reason });
      const { unmount } = render(<PublishSection available />);
      expect(await screen.findByTestId("publish-failed")).toHaveTextContent(`settings.publish.failed.${reason}`);
      expect(screen.getByTestId("publish-connect")).toHaveTextContent("settings.publish.retry");
      unmount();
    }
  });

  it("shows a translated reason when connecting fails outright", async () => {
    rpc.connectStart.mockRejectedValue({ data: { reason: "publish_unreachable" } });
    render(<PublishSection available />);
    await userEvent.click(await screen.findByTestId("publish-consent"));
    await userEvent.click(screen.getByTestId("publish-connect"));
    expect(await screen.findByTestId("publish-error")).toHaveTextContent("errors.publishUnreachable");
  });
});
