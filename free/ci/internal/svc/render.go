package svc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Files renders fixed service names to deterministic bytes, without side effects.
func Files(c Config) (map[string][]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	path, err := GatePath(c.CIHome)
	if err != nil {
		return nil, err
	}
	names := c.names()
	files := map[string][]byte{}
	if c.Manager == "launchd" {
		files[names[0]] = []byte(plist(c, path, false))
		files[names[1]] = []byte(plist(c, path, true))
	} else {
		files[names[0]] = []byte(unit(c, path, false))
		files[names[1]] = []byte(unit(c, path, true))
		files[names[2]] = []byte("[Unit]\nDescription=nSelf CI hourly selftest\n\n[Timer]\nOnCalendar=hourly\nPersistent=true\nUnit=nself-ci-selftest.service\n\n[Install]\nWantedBy=timers.target\n")
	}
	return files, nil
}

func arguments(selftest bool) []string {
	if selftest {
		return []string{"ci", "doctor", "--selftest", "--alert"}
	}
	return []string{"ci", "serve", "watch"}
}

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func plist(c Config, path string, selftest bool) string {
	label := "org.nself.ci-watch"
	if selftest {
		label = "org.nself.ci-selftest"
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n")
	entry := func(k, v string) { fmt.Fprintf(&b, "<key>%s</key><string>%s</string>\n", k, escapeXML(v)) }
	entry("Label", label)
	if c.System {
		entry("UserName", "nself-ci-watch")
		entry("GroupName", "nself-ci-watch")
	}
	b.WriteString("<key>ProgramArguments</key><array>\n")
	for _, a := range append([]string{c.Binary}, arguments(selftest)...) {
		fmt.Fprintf(&b, "<string>%s</string>\n", escapeXML(a))
	}
	b.WriteString("</array>\n<key>EnvironmentVariables</key><dict>\n")
	entry("PATH", path)
	entry("HOME", c.Home)
	entry("NSELF_CI_HOME", c.CIHome)
	entry("NSELF_CI_REPO", c.Repo)
	entry("NSELF_V15", "1")
	b.WriteString("</dict>\n")
	entry("WorkingDirectory", c.Repo)
	if selftest {
		b.WriteString("<key>StartInterval</key><integer>3600</integer>\n")
	} else {
		b.WriteString("<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><true/>\n<key>ThrottleInterval</key><integer>10</integer>\n")
		entry("StandardOutPath", filepath.Join(c.CIHome, "logs", "watch", "watch.log"))
		entry("StandardErrorPath", filepath.Join(c.CIHome, "logs", "watch", "watch.log"))
	}
	b.WriteString("</dict></plist>\n")
	return b.String()
}

// systemd performs specifier and command dollar expansion even inside quotes.
func systemdQuote(s string) string {
	return strconv.Quote(strings.ReplaceAll(s, "%", "%%"))
}

func unit(c Config, path string, selftest bool) string {
	kind, desc := "simple", "watcher"
	if selftest {
		kind, desc = "oneshot", "selftest"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=nSelf CI %s\nAfter=network-online.target\n\n[Service]\nType=%s\n", desc, kind)
	if c.System {
		b.WriteString("User=nself-ci-watch\nGroup=nself-ci-watch\n")
	}
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", systemdQuote(c.Repo))
	for _, e := range []string{"PATH=" + path, "HOME=" + c.Home, "NSELF_CI_HOME=" + c.CIHome, "NSELF_CI_REPO=" + c.Repo, "NSELF_V15=1"} {
		fmt.Fprintf(&b, "Environment=%s\n", systemdQuote(e))
	}
	if c.EnvFile != "" {
		fmt.Fprintf(&b, "EnvironmentFile=%s\n", systemdQuote(c.EnvFile))
	}
	fmt.Fprintf(&b, "ExecStart=%s %s\n", systemdQuote(strings.ReplaceAll(c.Binary, "$", "$$")), strings.Join(arguments(selftest), " "))
	b.WriteString("StandardOutput=journal\nStandardError=journal\nUMask=0077\n")
	if !selftest {
		b.WriteString("Restart=on-failure\nRestartSec=10\n\n[Install]\n")
		target := "default.target"
		if c.System {
			target = "multi-user.target"
		}
		fmt.Fprintf(&b, "WantedBy=%s\n", target)
	}
	return b.String()
}
