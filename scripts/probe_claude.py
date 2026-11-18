"""Exercise the installed Claude Code headless protocol using a disposable workspace."""
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
import uuid


def send(process, event):
    process.stdin.write(json.dumps(event) + "\n")
    process.stdin.flush()


@contextmanager
def stream(cli, workspace, options):
    args = [cli, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
            "--strict-mcp-config", "--no-chrome", "--disable-slash-commands", *options]
    # The probe uses the local Harness login. Service and project tokens have no role here.
    keep = {
        "PATH", "PATHEXT", "HOME", "USER", "LOGNAME", "USERPROFILE", "USERNAME",
        "APPDATA", "LOCALAPPDATA", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR",
        "LANG", "LC_ALL", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY",
        "XDG_CONFIG_HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_GIT_BASH_PATH",
        "SSL_CERT_FILE", "SSL_CERT_DIR", "COMSPEC", "HOMEDRIVE", "HOMEPATH",
    }
    env = {k: v for k, v in os.environ.items() if k.upper() in keep}
    with subprocess.Popen(args, cwd=workspace, env=env, stdin=subprocess.PIPE,
                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                          text=True, encoding="utf-8") as process:
        timer = threading.Timer(120, process.kill)
        timer.start()
        try:
            yield process
        finally:
            timer.cancel()
            if process.poll() is None:
                process.kill()


def turn(cli, workspace, session, prompt, resume):
    options = ["--permission-prompts", "host", "--permission-mode", "manual", "--permission-prompt-tool", "stdio",
               "--setting-sources", "user", "--settings", json.dumps({"permissions": {"ask": ["Write"]}, "disableAllHooks": True}),
               "--tools", "" if resume else "AskUserQuestion,Write", "--resume" if resume else "--session-id", session]
    with stream(cli, workspace, options) as process:
        events, result = [], None
        send(process, {"type": "user", "message": {"role": "user", "content": prompt},
                       "parent_tool_use_id": None, "session_id": session})
        for line in process.stdout:
            event = json.loads(line)
            if event["type"] == "control_request":
                request = event["request"]
                tool = request.get("tool_name")
                reply = {"behavior": "deny", "message": "Write permission denied for this probe."}
                if request.get("subtype") != "can_use_tool" or resume:
                    raise RuntimeError("Unexpected control request during the probe")
                if tool == "AskUserQuestion":
                    data = request["input"]
                    question, = data["questions"]
                    selected = next(o["label"] for o in question["options"] if "green" in o["label"].lower())
                    reply = {"behavior": "allow", "updatedInput": {
                        **data, "answers": {question["question"]: selected}}}
                if tool == "Write":
                    path = Path(request.get("input", {}).get("file_path", ""))
                    tool = "probe write" if (workspace / path).resolve() == workspace / "probe.txt" else "other write"
                events.append((tool, reply["behavior"]))
                send(process, {"type": "control_response", "response": {
                    "subtype": "success", "request_id": event["request_id"], "response": reply}})
            elif event["type"] == "result":
                if result is not None or event.get("is_error") or event.get("subtype") != "success" or event.get("session_id") != session:
                    raise RuntimeError("Claude did not complete the expected session")
                result = event.get("result")
                process.stdin.close()
        if process.wait() or not isinstance(result, str):
            raise RuntimeError("No successful Claude result; check the local login, version and connectivity")
        return process.pid, events, result.strip()


def interrupt_turn(cli, workspace, session):
    options = ["--restricted", "--safe-mode", "--permission-mode", "acceptEdits", "--tools", "",
               "--include-partial-messages", "--session-id", session]
    settings = Path(os.environ.get("CLAUDE_CONFIG_DIR", Path.home() / ".claude")) / "settings.json"
    if settings.is_file():
        options += ["--settings", str(settings)]
    request = str(uuid.uuid4())
    with stream(cli, workspace, options) as process:
        send(process, {"type": "user", "session_id": session, "parent_tool_use_id": None,
                       "message": {"role": "user", "content": "Write fifty separate sentences about the seasons. Use only text."}})
        started = sent = acknowledged = terminal = False
        for line in process.stdout:
            e = json.loads(line)
            kind = e["type"]
            if kind == "control_request":
                raise RuntimeError("Unexpected tool request in a text-only turn")
            if kind == "control_response":
                response = e["response"]
                if not sent or acknowledged or response["request_id"] != request or response["subtype"] != "success":
                    raise RuntimeError("Unexpected interrupt response")
                acknowledged = True
            elif e.get("session_id") == session:
                if kind == "system" and e.get("subtype") == "init":
                    started = True
                elif started and not sent and kind == "stream_event" and e["event"].get("delta", {}).get("type") == "text_delta":
                    send(process, {"type": "control_request", "request_id": request, "request": {"subtype": "interrupt"}})
                    sent = True
                elif kind == "result":
                    if not sent or not e.get("is_error") or e.get("terminal_reason") != "aborted_streaming":
                        raise RuntimeError("The native turn did not report interrupted streaming")
                    terminal = True
            if acknowledged and terminal:
                process.stdin.close()
                break
        if process.wait() != 1 or not (acknowledged and terminal):
            raise RuntimeError("Native interrupt lacked the expected receipt or process exit")
        return {"session_id": session, "pid": process.pid, "request_id": request, "exit_code": 1,
                "terminal_reason": "aborted_streaming"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin", default="claude", help="native Claude Code executable")
    parser.add_argument("--interrupt", action="store_true", help="interrupt an active text stream and verify the native receipt")
    args = parser.parse_args()
    binary = shutil.which(args.bin)
    if not binary or (os.name == "nt" and Path(binary).suffix.lower() != ".exe"):
        parser.error("supply the path to the native CLI executable with --bin")
    version = subprocess.check_output([binary, "--version"], text=True, timeout=10).strip()
    session = str(uuid.uuid4())
    marker = "FORGE-" + secrets.token_hex(8)
    with tempfile.TemporaryDirectory(prefix="forgeos-claude-") as tmp:
        workspace = Path(tmp)
        if args.interrupt:
            print(json.dumps({"version": version, **interrupt_turn(binary, workspace, session)}))
            return
        start, events, _ = turn(binary, workspace, session,
            f"The case identifier for this conversation is {marker}. Use AskUserQuestion to ask "
            "which color to use, Blue or Green. After the answer, use Write once with the exact "
            f"file_path {workspace / 'probe.txt'} and the selected color as content. If permission "
            "is denied, stop and report the selected color and denial.", False)
        answered = ("AskUserQuestion", "allow")
        if answered not in events or ("probe write", "deny") not in events[events.index(answered)+1:] or (workspace / "probe.txt").exists():
            raise RuntimeError(f"Expected an answered question and denied write; observed {events}")
        resumed, _, answer = turn(binary, workspace, session,
            "Return the case identifier from the previous message, then a space "
            "and the selected color. Output those two values only.", True)
        if answer.lower() != (marker + " Green").lower():
            raise RuntimeError("Resumed session did not recall the original value and selected color")
    print(json.dumps({"version": version, "session_id": session, "start_pid": start,
                      "resume_pid": resumed, "question_answered": True, "write_denied": True,
                      "workspace_write_observed": False, "native_resume": True}))


if __name__ == "__main__":
    main()
