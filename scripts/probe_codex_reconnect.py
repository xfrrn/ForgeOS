"""Private control-socket check for probe_codex_app_server.py --reconnect."""
from contextlib import contextmanager
import csv
import json
import os
import subprocess
import threading
import time
import uuid

from wsproto import ConnectionType, WSConnection
from wsproto.events import AcceptConnection, CloseConnection, Ping, Request, TextMessage

from probe_codex_app_server import Client, server_args, server_env


class SocketClient(Client):
    def __init__(self, process):
        super().__init__(process)
        self.ws = WSConnection(ConnectionType.CLIENT)
        self.events = []
        self.write(Request(host="localhost", target="/"))
        if not isinstance(self.event(), AcceptConnection):
            raise RuntimeError("Private socket rejected the WebSocket handshake")

    def write(self, event):
        self.process.stdin.write(self.ws.send(event))
        self.process.stdin.flush()

    def event(self):
        while not self.events:
            packet = os.read(self.process.stdout.fileno(), 65536)
            if not packet:
                raise RuntimeError("Private control socket closed")
            self.ws.receive_data(packet)
            self.events.extend(self.ws.events())
        return self.events.pop(0)

    def send(self, message):
        self.write(TextMessage(data=json.dumps(message)))

    def receive(self):
        parts = []
        while True:
            event = self.event()
            if isinstance(event, Ping):
                self.write(event.response())
            elif isinstance(event, CloseConnection):
                raise RuntimeError("App Server closed the private socket")
            elif isinstance(event, TextMessage):
                parts.append(event.data)
                if event.message_finished:
                    break
        message = json.loads("".join(parts))
        if message.get("method") in {"item/started", "turn/completed"}:
            self.notifications.append(message)
        return message


@contextmanager
def connection(binary, workspace, endpoint):
    # The native proxy transports raw WebSocket frames, including the initial HTTP upgrade.
    with subprocess.Popen([binary, "app-server", "proxy", "--sock", str(endpoint)], cwd=workspace,
                          env=server_env(), stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.DEVNULL) as proxy:
        timer = threading.Timer(120, proxy.kill)
        timer.start()
        try:
            client = SocketClient(proxy)
            client.call("initialize", {"clientInfo": {"name": "forgeos_probe", "title": "ForgeOS", "version": "1"}})
            client.send({"method": "initialized", "params": {}})
            yield client
        finally:
            timer.cancel()
            proxy.kill()


def reconnect(binary, root, marker, version):
    workspace, private = root / "work", root / "control"
    workspace.mkdir()
    if os.name == "nt":
        private.mkdir()
        sid = next(csv.reader(subprocess.check_output(["whoami", "/user", "/fo", "csv", "/nh"],
                                                       text=True).splitlines()))[1]
        subprocess.run(["icacls", str(private), "/inheritance:r", "/grant:r", f"*{sid}:(OI)(CI)(F)"],
                       check=True, stdout=subprocess.DEVNULL)
    else:
        private.mkdir(mode=0o700)
    endpoint = private / "s"
    with subprocess.Popen(server_args(binary, "unix://" + endpoint.as_posix()), cwd=workspace,
                          env=server_env(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                          stderr=subprocess.DEVNULL) as native:
        try:
            deadline = time.monotonic() + 10
            while not endpoint.exists():
                if native.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError("Private App Server socket unavailable")
                time.sleep(0.05)
            with connection(binary, workspace, endpoint) as first:
                thread = first.thread(workspace)
                command = "Start-Sleep -Seconds 22" if os.name == "nt" else "sleep 22"
                prompt = f"Run the command {command} once, then reply WAITED unless an updated answer arrives. Do not write files."
                turn = first.call("turn/start", {"threadId": thread, "input": [{"type": "text", "text": prompt}]})["turn"]["id"]
                while True:
                    if not first.notifications:
                        first.receive()
                        continue
                    event = first.notifications.pop(0)
                    params = event["params"]
                    if params.get("threadId") != thread:
                        continue
                    if (event["method"] == "item/started" and params.get("turnId") == turn
                            and params["item"]["type"] == "commandExecution"):
                        command_id = params["item"]["id"]
                        break
                    if event["method"] == "turn/completed" and params["turn"]["id"] == turn:
                        raise RuntimeError("Turn ended before its command started")
            with connection(binary, workspace, endpoint) as second:
                def read_turn():
                    history = second.call("thread/read", {"threadId": thread, "includeTurns": True})["thread"]
                    if history["id"] != thread or len(history["turns"]) != 1 or history["turns"][0]["id"] != turn:
                        raise RuntimeError("Original thread and turn no longer match")
                    return history["turns"][0]

                if read_turn()["status"] != "inProgress" or second.thread(workspace, existing=thread) != thread:
                    raise RuntimeError("Original turn was not active at reconnection")
                key = str(uuid.uuid4())
                instruction = f"After the command finishes, reply with only the following token on its own line:\n{marker}"
                response = second.call("turn/steer", {"threadId": thread, "expectedTurnId": turn, "clientUserMessageId": key,
                                       "input": [{"type": "text", "text": instruction}]})
                if response["turnId"] != turn:
                    raise RuntimeError("Reconnected input reached another turn")
                deadline = time.monotonic() + 90
                current = read_turn()
                while current["status"] == "inProgress" and time.monotonic() < deadline:
                    time.sleep(0.5)
                    current = read_turn()
                replies = [i["text"].strip() for i in current["items"] if i["type"] == "agentMessage"]
                inputs = [i for i in current["items"] if i["type"] == "userMessage"
                          and any(block.get("text") == instruction for block in i.get("content", []))]
                commands = [item for item in current["items"] if item["type"] == "commandExecution" and item["id"] == command_id]
                if len(commands) != 1 or commands[0]["status"] != "completed" or commands[0].get("exitCode") != 0:
                    raise RuntimeError("The command active at disconnect did not finish successfully")
                if current["status"] != "completed" or not replies or replies[-1] != marker:
                    raise RuntimeError("Reconnected turn did not answer the new instruction")
                if len(inputs) != 1 or inputs[0].get("clientId") != key:
                    raise RuntimeError("Reconnected turn did not record exactly one matching input")
                if native.poll() is not None or any(workspace.iterdir()):
                    raise RuntimeError("Server exited or its read-only workspace changed")
                return {"version": version, "pid": native.pid, "thread": thread, "turn": turn, "input_item": inputs[0]["id"],
                        "command_item": command_id, "command_exit": commands[0]["exitCode"], "client_id": key,
                        "history_client_id": inputs[0]["clientId"], "reply": marker, "status": current["status"],
                        "sandbox": "read-only"}
        finally:
            native.kill()
