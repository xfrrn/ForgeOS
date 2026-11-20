"""Sign in through the workbench form with an isolated test account."""
import os

from playwright.sync_api import Error


def login_to_workbench(page):
    email = os.environ.get("FORGEOS_TEST_OWNER_EMAIL")
    password = os.environ.get("FORGEOS_TEST_OWNER_PASSWORD")
    if not email or not password:
        raise RuntimeError("Set FORGEOS_TEST_OWNER_EMAIL and FORGEOS_TEST_OWNER_PASSWORD")

    try:
        page.get_by_label("邮箱", exact=True).fill(email)
        field = page.get_by_label("密码", exact=True)
        field.fill(password)
        with page.expect_response(
            lambda response: response.url.endswith("/auth/login") and response.request.method == "POST",
        ) as signed_in:
            field.press("Enter")
    except Error:
        raise RuntimeError("Could not submit the workbench login form") from None
    response = signed_in.value
    if not response.ok:
        raise RuntimeError(f"Workbench login returned HTTP {response.status}")
