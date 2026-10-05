#!/usr/bin/env python3
"""Scripted loopback stand-in for the Hetzner Cloud API (P7-CANON-12 parity).

Usage: parity-fixture.py PORT LOGFILE
Every request is appended to LOGFILE as "METHOD path?query body" (body as sorted-key JSON).
The scenario is chosen by the server id in the path:
  1 happy path        2 snapshot start fails (500)   3 snapshot action reports error
  4 snapshot never available (image stays creating)  5 primary IP read fails (500)
  6 primary IP write fails on the ipv6 address       7 server unknown (404)
  8 image poll fails (500)                           9 DELETE fails (500)
  10 server with no primary IPs
Ids: image 1000+n, action 2000+n, primary IPs n*10 (ipv4) and n*10+1 (ipv6).
It never forwards anything and binds loopback only.
"""
import http.server, json, re, socketserver, sys

PORT, LOG = int(sys.argv[1]), sys.argv[2]
TYPES = [{"name": "cx11", "cores": 1, "memory": 2, "disk": 20}, {"name": "cx22", "cores": 2, "memory": 4, "disk": 40},
         {"name": "cx41", "cores": 4, "memory": 16, "disk": 160}]


def server(n, name=None, typ="cx22"):
    ips = n != 10
    return {"id": n, "name": name or "srv-%d" % n, "status": "running", "server_type": [t for t in TYPES if t["name"] == typ][0],
            "datacenter": {"location": {"name": "fsn1"}},
            "public_net": {"ipv4": {"id": n * 10 if ips else 0, "ip": "203.0.113.%d" % n},
                           "ipv6": {"id": n * 10 + 1 if ips else 0, "ip": "2001:db8::%d" % n}},
            "labels": {"managed-by": "nself-cli"}}


class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def reply(self, code, obj=None):
        data = b"" if obj is None else json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def err(self, code, msg="scripted failure"):
        self.reply(code, {"error": {"code": "scripted", "message": msg}})

    def handle_any(self):
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n).decode() if n else ""
        try:
            body = json.dumps(json.loads(raw), sort_keys=True) if raw else ""
        except ValueError:
            body = raw
        with open(LOG, "a") as f:
            f.write("%s %s %s\n" % (self.command, self.path, body))
        p = self.path.split("?")[0]
        if not p.startswith("/v1/"):
            return self.err(404, "bad prefix")
        p = p[3:]
        m = self.command
        if m == "POST" and p == "/servers":
            req = json.loads(raw or "{}")
            if req.get("name") == "fail":
                return self.err(422, "name rejected")
            s = server(77, req.get("name"), "cx22")
            s["labels"] = req.get("labels", {})
            return self.reply(201, {"server": s})
        if m == "GET" and p == "/servers":
            return self.reply(200, {"servers": [server(1), server(2)]})
        if m == "GET" and p == "/server_types":
            return self.reply(200, {"server_types": TYPES})
        r = re.match(r"^/servers/(\d+)(/actions/(\w+))?$", p)
        if r:
            n, act = int(r.group(1)), r.group(3)
            if n == 7 and m == "GET":
                return self.err(404, "server not found")
            if m == "GET" and not act:
                return self.reply(200, {"server": server(n)})
            if m == "DELETE":
                return self.err(500) if n == 9 else self.reply(200, {"action": {"id": 3000 + n, "status": "success"}})
            if act == "create_image":
                if n == 2:
                    return self.err(500)
                st = "error" if n == 3 else "running"
                a = {"id": 2000 + n, "status": st}
                if n == 3:
                    a["error"] = {"code": "scripted", "message": "snapshot failed"}
                return self.reply(201, {"image": {"id": 1000 + n, "type": "snapshot", "status": "creating"}, "action": a})
            if act == "change_type":
                return self.reply(201, {"action": {"id": 4000 + n, "status": "running", "command": "change_server_type"}})
        r = re.match(r"^/images/(\d+)$", p)
        if r:
            n = int(r.group(1)) - 1000
            if n == 8:
                return self.err(500)
            return self.reply(200, {"image": {"id": n + 1000, "status": "creating" if n == 4 else "available"}})
        r = re.match(r"^/actions/(\d+)$", p)
        if r:
            n = int(r.group(1)) - 2000
            return self.reply(200, {"action": {"id": n + 2000, "status": "running" if n == 4 else "success"}})
        r = re.match(r"^/primary_ips/(\d+)$", p)
        if r:
            ip = int(r.group(1)); n = ip // 10
            if n == 5 and m == "GET":
                return self.err(500)
            if n == 6 and ip % 10 == 1 and m == "PUT":
                return self.err(500)
            if m == "GET":
                return self.reply(200, {"primary_ip": {"id": ip, "ip": "198.51.100.%d" % (ip % 100), "type": "ipv4" if ip % 10 == 0 else "ipv6",
                                                       "assignee_id": n, "auto_delete": True}})
            if m == "PUT":
                return self.reply(200, {"primary_ip": {"id": ip}})
        return self.err(404, "unscripted " + m + " " + p)

    do_GET = do_POST = do_PUT = do_DELETE = handle_any


class S(socketserver.ThreadingMixIn, http.server.HTTPServer):
    allow_reuse_address = True
    daemon_threads = True


S(("127.0.0.1", PORT), H).serve_forever()
