"""Check the installed Codex App Server over its local stdio transport."""
import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import threading


class Client:
    def __init__(self, process):
        self.process = process
        self.sequence = 0
        self.notifications = []

    def send(self, message):
        self.process.stdin.write(json.dumps(message) + "\n")
        self.process.stdin.flush()

    def receive(self):
        line = self.process.stdout.readline()
        if not line:
            raise RuntimeError("App Server closed the protocol stream")
        msg = json.loads(line)
        if ("method" in msg and "id" in msg) or msg.get("method") in {"turn/started", "turn/completed", "item/completed"}:
            self.notifications.append(msg)
        return msg

    def call(self, method, params):
        self.sequence += 1
        self.send({"id": self.sequence, "method": method, "params": params})
        while True:
            msg = self.receive()
            if "method" not in msg and msg.get("id") == self.sequence:
                if "error" in msg:
                    raise RuntimeError(f"{method} failed with protocol error {msg['error'].get('code')}")
                return msg["result"]

    def thread(self, workspace, existing=None, approval="never"):
        params = {"cwd": str(workspace), "sandbox": "read-only", "approvalPolicy": approval}
        if existing:
            params["threadId"] = existing
        result = self.call("thread/resume" if existing else "thread/start", params)
        if result["sandbox"]["type"] != "readOnly" or result["approvalPolicy"] != approval:
            raise RuntimeError("App Server selected an unexpected sandbox or approval policy")
        thread = result["thread"]["id"]
        if existing and existing != thread:
            raise RuntimeError("App Server resumed a different thread")
        return thread

    def turn(self, thread, prompt, interrupt=False, decision=None, workspace=None, command=None):
        self.notifications.clear()
        turn = self.call("turn/start", {"threadId": thread, "input": [{"type": "text", "text": prompt}]})["turn"]["id"]
        message, started = "", False
        self.approval = None
        while True:
            if not self.notifications:
                self.receive()
                continue
            event = self.notifications.pop(0)
            params = event["params"]
            if "id" in event:
                if (decision not in ("decline", "cancel", "accept") or self.approval is not None
                        or event["method"] != "item/commandExecution/requestApproval"
                        or params.get("threadId") != thread or params.get("turnId") != turn
                        or not params.get("itemId")):
                    raise RuntimeError(f"Unexpected server request: {event['method']}")
                if (Path(params.get("cwd") or "").resolve() != workspace
                        or params.get("additionalPermissions") or params.get("networkApprovalContext")
                        or params.get("commandActions") != [{"type": "unknown", "command": command}]):
                    raise RuntimeError("Approval did not match the temporary probe command")
                self.approval = {"request": event["id"], "item": params["itemId"], "decision": decision}
                self.send({"id": event["id"], "result": {"decision": decision}})
                continue
            if params.get("threadId") != thread:
                continue
            method = event["method"]
            if method == "item/completed" and params.get("turnId") == turn:
                item = params["item"]
                if item["type"] == "agentMessage":
                    message = item["text"].strip()
                if self.approval and item["type"] == "commandExecution" and item["id"] == self.approval["item"]:
                    self.approval["status"] = item["status"]
                    if decision == "accept":
                        self.approval["output"] = (item.get("aggregatedOutput") or "").strip()
            if method == "turn/started" and params["turn"]["id"] == turn:
                if interrupt and not started:
                    self.call("turn/interrupt", {"threadId": thread, "turnId": turn})
                started = True
            if method == "turn/completed" and params["turn"]["id"] == turn:
                expected = "interrupted" if interrupt or decision == "cancel" else "completed"
                if not started or params["turn"]["status"] != expected:
                    raise RuntimeError(f"Expected a started then {expected} turn, got {params['turn']['status']}")
                command_status = "completed" if decision == "accept" else "declined"
                if decision and (not self.approval or self.approval.get("status") != command_status):
                    raise RuntimeError(f"The turn ended without a {command_status} command receipt")
                return turn, message


@contextmanager
def server(binary, workspace):
    args = [binary, "app-server", "--listen", "stdio://", "--disable", "apps",
            "--disable", "plugins", "--disable", "hooks", "-c", "mcp_servers={}",
            "-c", 'web_search="disabled"']
    if os.name == "nt":
        args += ["-c", 'windows.sandbox="unelevated"']
    keep = {
        "PATH", "PATHEXT", "HOME", "USER", "LOGNAME", "USERPROFILE", "USERNAME",
        "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA", "SYSTEMROOT", "WINDIR",
        "COMSPEC", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "CODEX_HOME", "HTTP_PROXY",
        "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR",
        "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR",
    }
    env = {k: v for k, v in os.environ.items() if k.upper() in keep}
    with subprocess.Popen(args, cwd=workspace, env=env, stdin=subprocess.PIPE,
                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                          text=True, encoding="utf-8") as proc:
        timer = threading.Timer(120, proc.kill)
        timer.start()
        try:
            client = Client(proc)
            client.call("initialize", {"clientInfo": {"name": "forgeos_probe", "title": "ForgeOS", "version": "1"}})
            client.send({"method": "initialized", "params": {}})
            yield client
        finally:
            timer.cancel()
            proc.stdin.close()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin", default="codex", help="native Codex executable")
    parser.add_argument("--approvals", action="store_true", help="exercise one-time approval, decline and cancel")
    args = parser.parse_args()
    binary = shutil.which(args.bin)
    if not binary or (os.name == "nt" and Path(binary).suffix.lower() != ".exe"):
        parser.error("supply the path to the native CLI executable with --bin")
    version = subprocess.check_output([binary, "--version"], text=True, timeout=10).strip()
    marker = "FORGE-" + secrets.token_hex(8)
    with tempfile.TemporaryDirectory(prefix="forgeos-app-server-") as tmp:
        workspace = Path(tmp)
        if args.approvals:
            # The fixture has no external dependencies. Its only effect would be visible here.
            (workspace / "approval_probe.py").write_text(
                "from pathlib import Path\nPath('executed.txt').write_text('ran')\n", encoding="utf-8")
            receipts = []
            with server(binary, workspace) as client:
                thread = client.thread(workspace, approval="untrusted")
                stamp = str(secrets.randbelow(10**12))
                read_command = f'python -c "print({stamp})"'
                for decision, command in (("accept", read_command), ("decline", read_command),
                                          ("decline", "python approval_probe.py"), ("cancel", "python approval_probe.py")):
                    turn, _ = client.turn(thread, f"Run this exact command in the current directory once: {command}\n"
                                          "Use your shell tool. We are testing the native "
                                          "approval UI. If permission is declined, stop without retrying "
                                          "or using another tool.", decision=decision, workspace=workspace, command=command)
                    if decision == "accept" and client.approval["output"] != stamp:
                        raise RuntimeError("Approved command did not return the expected value")
                    if (workspace / "executed.txt").exists():
                        raise RuntimeError("Denied command executed")
                    receipts.append({"thread": thread, "turn": turn, **client.approval})
            print(json.dumps({"version": version, "sandbox": "read-only", "approvals": receipts}))
            return
        with server(binary, workspace) as first:
            thread = first.thread(workspace)
            start, answer = first.turn(thread, f"Test case {marker}. Do not use tools. Reply only OK.")
            if answer != "OK":
                raise RuntimeError("Unexpected first turn response")
            first_pid = first.process.pid
        with server(binary, workspace) as second:
            second.thread(workspace, thread)
            resumed, answer = second.turn(thread, "Return only the test case identifier from my last message. Do not use tools.")
            if answer != marker:
                raise RuntimeError("Resumed thread did not recall the case identifier")
            interrupted, _ = second.turn(thread, "Write ten sentences about the months. Do not use tools.", interrupt=True)
            second_pid = second.process.pid
    print(json.dumps({"version": version, "thread_id": thread, "start_pid": first_pid,
                      "resume_pid": second_pid, "completed_turns": [start, resumed],
                      "interrupted_turn": interrupted, "sandbox": "read-only"}))


if __name__ == "__main__":
    main()
