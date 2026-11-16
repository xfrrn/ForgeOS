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
        if "method" in msg and "id" in msg:
            raise RuntimeError("Unexpected server request during the read-only probe")
        if msg.get("method") in {"turn/started", "turn/completed", "item/completed"}:
            self.notifications.append(msg)
        return msg

    def call(self, method, params):
        self.sequence += 1
        self.send({"id": self.sequence, "method": method, "params": params})
        while True:
            msg = self.receive()
            if msg.get("id") == self.sequence:
                if "error" in msg:
                    raise RuntimeError(f"{method} failed with protocol error {msg['error'].get('code')}")
                return msg["result"]

    def thread(self, workspace, existing=None):
        params = {"cwd": str(workspace), "sandbox": "read-only", "approvalPolicy": "never"}
        if existing:
            params["threadId"] = existing
        result = self.call("thread/resume" if existing else "thread/start", params)
        if result["sandbox"]["type"] != "readOnly" or result["approvalPolicy"] != "never":
            raise RuntimeError("App Server selected an unexpected sandbox or approval policy")
        thread = result["thread"]["id"]
        if existing and existing != thread:
            raise RuntimeError("App Server resumed a different thread")
        return thread

    def turn(self, thread, prompt, interrupt=False):
        self.notifications.clear()
        turn = self.call("turn/start", {"threadId": thread, "input": [{"type": "text", "text": prompt}]})["turn"]["id"]
        message, started = "", False
        while True:
            if not self.notifications:
                self.receive()
                continue
            event = self.notifications.pop(0)
            params = event["params"]
            if params.get("threadId") != thread:
                continue
            method = event["method"]
            if method == "item/completed" and params.get("turnId") == turn and params["item"]["type"] == "agentMessage":
                message = params["item"]["text"].strip()
            if method == "turn/started" and params["turn"]["id"] == turn:
                if interrupt and not started:
                    self.call("turn/interrupt", {"threadId": thread, "turnId": turn})
                started = True
            if method == "turn/completed" and params["turn"]["id"] == turn:
                expected = "interrupted" if interrupt else "completed"
                if not started or params["turn"]["status"] != expected:
                    raise RuntimeError(f"Expected a started then {expected} turn")
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
    binary = shutil.which(parser.parse_args().bin)
    if not binary or (os.name == "nt" and Path(binary).suffix.lower() != ".exe"):
        parser.error("supply the path to the native CLI executable with --bin")
    version = subprocess.check_output([binary, "--version"], text=True, timeout=10).strip()
    marker = "FORGE-" + secrets.token_hex(8)
    with tempfile.TemporaryDirectory(prefix="forgeos-app-server-") as tmp:
        workspace = Path(tmp)
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
