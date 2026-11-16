"""Exercise the installed Claude Code headless protocol using a disposable workspace."""
import argparse
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


def turn(cli, workspace, session, prompt, resume):
    args = [
        cli, "-p", "--input-format", "stream-json", "--output-format", "stream-json",
        "--verbose", "--permission-prompts", "host", "--permission-mode", "manual",
        "--permission-prompt-tool", "stdio", "--strict-mcp-config", "--no-chrome",
        "--disable-slash-commands", "--setting-sources", "user",
        "--settings", json.dumps({"permissions": {"ask": ["Write"]}, "disableAllHooks": True}),
        "--tools", "" if resume else "AskUserQuestion,Write",
        "--resume" if resume else "--session-id", session,
    ]
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
        events = []
        result = None
        try:
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
        finally:
            timer.cancel()
            if process.poll() is None:
                process.kill()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bin", default="claude", help="native Claude Code executable")
    binary = shutil.which(parser.parse_args().bin)
    if not binary or (os.name == "nt" and Path(binary).suffix.lower() != ".exe"):
        parser.error("supply the path to the native CLI executable with --bin")
    version = subprocess.check_output([binary, "--version"], text=True, timeout=10).strip()
    session = str(uuid.uuid4())
    marker = "FORGE-" + secrets.token_hex(8)
    with tempfile.TemporaryDirectory(prefix="forgeos-claude-") as tmp:
        workspace = Path(tmp)
        start, events, _ = turn(binary, workspace, session,
            f"The case identifier for this conversation is {marker}. Use AskUserQuestion to ask "
            "which color to use, Blue or Green. After the answer, use Write once to create "
            "probe.txt in your current working directory with the selected color. If permission "
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
