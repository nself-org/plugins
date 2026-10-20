package darwin

// Purpose: the LaunchDaemon of pull mode (Epic D20) and the scripts that install, load and remove it.
// The plist is generated here as data (a golden test pins the text), copied to the node, linted with
// `plutil -lint` and installed with `install -m 0644 -o root -g wheel`; `launchctl bootstrap system`
// loads it.
// Inputs:  none for the plist (fixed layout); the script texts take no input either.
// Outputs: Plist() text and three scripts.
// Constraints: the service runs as `nself-ci`, never root, with HOME set to its private state directory (the
//          account's own home is /var/empty, and a HOME inherited from an admin shell once made the start
//          guard read that admin's files); RunAtLoad plus a KeepAlive tied to the
//          enrollment file means launchd starts `pull` once at load and keeps it up only after
//          `nself-ci-agent join` has written <state>/pki/enrollment.json (no crash loop on a node that
//          has not joined); a changed plist is booted out and bootstrapped again, an unchanged one is
//          left running; logs go under /var/log/nself-ci-agent.

import "fmt"

// PlistStage is the file name, in the ssh user's home, the plist is copied to before it is installed.
const PlistStage = ".nself-ci-agent.plist.stage"

// Plist is the text of the LaunchDaemon.
func Plist() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>UserName</key>
	<string>%[2]s</string>
	<key>GroupName</key>
	<string>%[2]s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%[3]s</string>
		<string>pull</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>HOME</key>
		<string>%[4]s</string>
		<key>NSELF_CI_AGENT_HOME</key>
		<string>%[4]s</string>
		<key>NSELF_CI_AGENT_WORK</key>
		<string>%[5]s</string>
		<key>PATH</key>
		<string>/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:/opt/homebrew/bin</string>
	</dict>
	<key>WorkingDirectory</key>
	<string>%[5]s</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>PathState</key>
		<dict>
			<key>%[7]s</key>
			<true/>
		</dict>
	</dict>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>%[6]s/agent.log</string>
	<key>StandardErrorPath</key>
	<string>%[6]s/agent.log</string>
</dict>
</plist>
`, Label, agentUser, agentPath, StateDir, WorkDir, logDir, EnrollmentFile)
}

// plistScript lints the staged plist, installs it and (re)loads the service when it is new or changed.
func plistScript() string {
	return fmt.Sprintf(scriptHead+`trap 'rm -f ./%[1]s' EXIT
plutil -lint ./%[1]s >/dev/null
changed=1
if [ -e %[2]s ] && cmp -s ./%[1]s %[2]s; then changed=0; fi
install -o root -g wheel -m 0644 ./%[1]s %[2]s
rm -f ./%[1]s
loaded=0
if launchctl print system/%[3]s >/dev/null 2>&1; then loaded=1; fi
if [ "$loaded" = 1 ] && [ "$changed" = 1 ]; then
  launchctl bootout system/%[3]s >/dev/null 2>&1 || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do launchctl print system/%[3]s >/dev/null 2>&1 || break; sleep 1; done
  loaded=0
fi
if [ "$loaded" = 0 ]; then launchctl bootstrap system %[2]s; fi
launchctl print system/%[3]s >/dev/null
`, PlistStage, PlistPath, Label)
}

// bootoutScript unloads the service when it is loaded and removes its plist.
func bootoutScript() string {
	return fmt.Sprintf(scriptHead+`if launchctl print system/%[1]s >/dev/null 2>&1; then launchctl bootout system/%[1]s >/dev/null 2>&1 || true; fi
for _ in 1 2 3 4 5 6 7 8 9 10; do launchctl print system/%[1]s >/dev/null 2>&1 || break; sleep 1; done
rm -f %[2]s
if launchctl print system/%[1]s >/dev/null 2>&1; then echo 'the service is still loaded' >&2; exit 1; fi
`, Label, PlistPath)
}
