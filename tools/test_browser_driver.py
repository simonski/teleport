"""Exercise real pointer/keyboard actions, CSP-safe waits and script completion."""
import json
import os
import subprocess
import sys
import unittest

from browser_driver import Browser
from graph_browser import verify_camera_retained


class BrowserDriverTest(unittest.TestCase):
    def test_camera_check_accepts_roundoff_but_rejects_movement_and_selection_loss(self):
        before = {"selected":"ticket:1", "camera":[372.9752042913831,443,995], "target":[-159.99999999999997,70,-70]}
        after = {"selected":"ticket:1", "camera":[372.97520429138314,443,995], "target":[-159.99999999999994,70,-70]}
        def browser(command, script):
            return json.dumps(json.dumps(before)) if script == "window.__graphCamera" else json.dumps(after)
        verify_camera_retained(browser)
        after["camera"][0] += 1
        with self.assertRaises(AssertionError):
            verify_camera_retained(browser)
        after["camera"] = before["camera"]
        after["selected"] = "ticket:2"
        with self.assertRaises(AssertionError):
            verify_camera_retained(browser)

    def test_real_actions_and_function_assignment_preserve_script_semantics(self):
        browser = Browser("teleport-test-driver", dict(os.environ))
        try:
            browser("open", "about:blank")
            browser.page.set_content('<meta http-equiv="Content-Security-Policy" content="script-src \'self\'"><label>Name<input id="name"></label><button>Add</button><output id="result"></output>')
            browser("eval", "window.calls=0;window.callback=()=>++window.calls")
            self.assertEqual(json.loads(browser("eval", "window.calls")), 0)
            browser("eval", "document.querySelector('button').onclick=()=>{window.calls++;document.querySelector('#result').textContent=document.querySelector('#name').value}")
            browser("find", "label", "Name", "fill", "fixture")
            browser("find", "role", "button", "click", "--name", "Add", "--exact")
            browser("wait", "--fn", "document.querySelector('#result').textContent==='fixture'")
            self.assertEqual(json.loads(browser("eval", "window.calls")), 1)
            self.assertEqual(json.loads(browser("eval", "Promise.resolve(42)")), 42)
            with self.assertRaisesRegex(RuntimeError, "deliberate fixture failure"):
                browser("eval", "throw new Error('deliberate fixture failure')")
        finally:
            browser("close")


if __name__ == "__main__":
    if "--worker-tests" in sys.argv:
        sys.argv.remove("--worker-tests")
        unittest.main()
    else:
        from browser_setup import browser_python
        from tool_history import run
        def main():
            try:
                executable = browser_python()
            except SystemExit:
                print("SKIP: real browser driver tests require make browser-tools")
                return
            subprocess.run([executable, "-B", __file__, "--worker-tests", *sys.argv[1:]], check=True, timeout=15)
        run(main)
