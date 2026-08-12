import http.server
import socketserver
import urllib.request
import urllib.parse
import json
import os
import sys
import ssl
import datetime
import webbrowser
import threading

# ============================================================
#  버전 및 PORT 설정
#  - HTTPS_PORT (8080): 네이버 OAuth 리다이렉트 수신 전용
#  - HTTP_PORT  (8081): OBS 독 위젯 서빙 + API 프록시
# ============================================================
APP_VERSION = "v0.1.1"
APP_NAME = "ChzzkObsDockServer"
REG_PATH = r"Software\Microsoft\Windows\CurrentVersion\Run"
HTTPS_PORT = 8080
HTTP_PORT = 8081
CONFIG_FILE = "config.json"
CERT_FILE = "cert.pem"
KEY_FILE = "key.pem"
USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"


def resource_path(filename):
    """Return the path for a bundled resource or a source-tree file."""
    base_path = getattr(sys, "_MEIPASS", os.path.dirname(os.path.abspath(__file__)))
    return os.path.join(base_path, filename)


def ensure_ssl_cert():
    if not os.path.exists(CERT_FILE) or not os.path.exists(KEY_FILE):
        print("[SSL] Self-signed 인증서 생성 중...")
        try:
            from cryptography import x509
            from cryptography.x509.oid import NameOID
            from cryptography.hazmat.primitives import hashes, serialization
            from cryptography.hazmat.primitives.asymmetric import rsa

            key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
            subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, u"localhost")])
            cert = (
                x509.CertificateBuilder()
                .subject_name(subject)
                .issuer_name(issuer)
                .public_key(key.public_key())
                .serial_number(x509.random_serial_number())
                .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
                .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=3650))
                .add_extension(
                    x509.SubjectAlternativeName([x509.DNSName(u"localhost"), x509.DNSName(u"127.0.0.1")]),
                    critical=False,
                )
                .sign(key, hashes.SHA256())
            )
            with open(KEY_FILE, "wb") as f:
                f.write(key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.TraditionalOpenSSL, serialization.NoEncryption()))
            with open(CERT_FILE, "wb") as f:
                f.write(cert.public_bytes(serialization.Encoding.PEM))
            print("[SSL] 인증서 생성 완료!")
        except Exception as e:
            print("[SSL Error]", e)


ensure_ssl_cert()

# ============================================================
#  Config
# ============================================================
config = {
    "client_id": "",
    "client_secret": "",
    "redirect_uri": "https://localhost:8080",
    "access_token": "",
    "refresh_token": "",
}

if os.path.exists(CONFIG_FILE):
    try:
        with open(CONFIG_FILE, "r", encoding="utf-8") as f:
            config.update(json.load(f))
    except Exception as e:
        print("[Config] load error:", e)


def save_config():
    with open(CONFIG_FILE, "w", encoding="utf-8") as f:
        json.dump(config, f, ensure_ascii=False, indent=2)


def random_state():
    import random, string
    return "".join(random.choices(string.ascii_lowercase + string.digits, k=12))


def exchange_token(code, state):
    url = "https://openapi.chzzk.naver.com/auth/v1/token"
    redirect_uri = config.get("redirect_uri", "https://localhost:8080")
    payload = json.dumps({
        "grantType": "authorization_code",
        "clientId": config["client_id"],
        "clientSecret": config["client_secret"],
        "code": code,
        "state": state,
        "redirectUri": redirect_uri,
    }).encode()
    req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json", "User-Agent": USER_AGENT}, method="POST")
    try:
        with urllib.request.urlopen(req) as resp:
            data = json.loads(resp.read().decode())
            return data.get("content") or data.get("data") or data
    except Exception as e:
        print("[Token Exchange Error]:", e.read().decode() if hasattr(e, "read") else e)
        return None


def refresh_access_token():
    url = "https://openapi.chzzk.naver.com/auth/v1/token"
    payload = json.dumps({
        "grantType": "refresh_token",
        "clientId": config["client_id"],
        "clientSecret": config["client_secret"],
        "refreshToken": config.get("refresh_token"),
    }).encode()
    req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json", "User-Agent": USER_AGENT}, method="POST")
    try:
        with urllib.request.urlopen(req) as resp:
            data = json.loads(resp.read().decode())
            res = data.get("content") or data.get("data") or data
            if res and "accessToken" in res:
                config["access_token"] = res["accessToken"]
                if "refreshToken" in res:
                    config["refresh_token"] = res["refreshToken"]
                save_config()
                print("[Refresh] 토큰 갱신 성공!")
                return True
    except Exception as e:
        print("[Refresh Error]:", e)
    return False


# ============================================================
#  공용 핸들러 로직
# ============================================================
class CommonHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):
        """Avoid writing request logs when the packaged app has no console."""
        pass

    def handle(self):
        try:
            super().handle()
        except Exception:
            pass

    def end_headers(self):
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        super().end_headers()

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def send_json(self, data, status=200):
        body = json.dumps(data, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def send_html(self, status, html):
        body = html.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def proxy_request(self, method, path, body=None):
        is_category_search = "/categories/search" in path
        token = config.get("access_token")
        if not token and not is_category_search:
            self.send_json({"code": 401, "message": "Access Token 미설정"}, 401)
            return

        url = f"https://openapi.chzzk.naver.com{path}"
        headers = {"User-Agent": USER_AGENT, "Content-Type": "application/json"}
        if is_category_search:
            # 카테고리 검색은 Client Credentials 인증 사용
            headers["Client-Id"] = config.get("client_id", "")
            headers["Client-Secret"] = config.get("client_secret", "")
        else:
            headers["Authorization"] = f"Bearer {token}"

        req = urllib.request.Request(url, data=body, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req) as resp:
                rb = resp.read()
                self.send_response(resp.status)
                self.send_header("Content-Type", resp.headers.get("Content-Type", "application/json"))
                self.send_header("Content-Length", str(len(rb)))
                self.end_headers()
                self.wfile.write(rb)
        except urllib.error.HTTPError as e:
            eb = e.read()
            if e.code == 401 and config.get("refresh_token") and not is_public:
                if refresh_access_token():
                    return self.proxy_request(method, path, body)
            self.send_response(e.code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(eb)))
            self.end_headers()
            self.wfile.write(eb)
        except Exception as e:
            err = json.dumps({"code": 500, "message": str(e)}, ensure_ascii=False).encode()
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(err)))
            self.end_headers()
            self.wfile.write(err)

    def handle_oauth_callback(self, code, state):
        """OAuth 코드로 토큰 교환 → HTML 결과 페이지 반환"""
        token_data = exchange_token(code, state)
        if token_data and "accessToken" in token_data:
            config["access_token"] = token_data["accessToken"]
            if "refreshToken" in token_data:
                config["refresh_token"] = token_data["refreshToken"]
            save_config()
            print(f"[OAuth] 토큰 발급 성공!")
            self.send_html(200, SUCCESS_HTML)
        else:
            print("[OAuth] 토큰 교환 실패")
            self.send_html(400, FAIL_HTML)


# ============================================================
#  HTTPS 핸들러 (포트 8080) — 네이버 OAuth 콜백만 처리
# ============================================================
class HttpsOAuthHandler(CommonHandler):
    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        query = urllib.parse.parse_qs(parsed.query)

        if "code" in query:
            self.handle_oauth_callback(query["code"][0], query.get("state", [""])[0])
            return

        # HTTPS 포트로 직접 접속한 경우 → HTTP 포트로 안내
        self.send_html(200, f"""<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>CHZZK Server</title>
<style>body{{background:#0B0E11;color:#A8B3C7;font-family:system-ui;text-align:center;padding:60px 20px}}
.card{{background:#141921;border:1px solid #1E2630;border-radius:16px;max-width:420px;margin:0 auto;padding:32px}}
h2{{color:#fff;font-size:20px;margin-bottom:12px}}
code{{background:#1E2630;padding:4px 10px;border-radius:6px;color:#00FFA3;font-size:14px}}
p{{font-size:13px;line-height:1.7}}</style></head>
<body><div class="card">
<h2>✅ CHZZK HTTPS 서버 정상 작동</h2>
<p>이 포트(8080)는 네이버 OAuth 콜백 전용입니다.<br>
OBS 독 위젯은 아래 주소를 사용하세요:</p>
<p><code>http://localhost:{HTTP_PORT}</code></p>
</div></body></html>""")


# ============================================================
#  HTTP 핸들러 (포트 8081) — OBS 독 위젯 + API 프록시
# ============================================================
class HttpDockHandler(CommonHandler):
    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        query = urllib.parse.parse_qs(parsed.query)

        if parsed.path == "/config":
            self.send_json(config)
            return

        if parsed.path == "/start-oauth":
            cid = config.get("client_id")
            ruri = config.get("redirect_uri", "https://localhost:8080")
            st = random_state()
            auth_url = f"https://chzzk.naver.com/account-interlock?clientId={urllib.parse.quote(cid)}&redirectUri={urllib.parse.quote(ruri)}&state={urllib.parse.quote(st)}"
            print(f"[OAuth] 브라우저 실행: {auth_url}")
            webbrowser.open(auth_url)
            self.send_json({"status": "ok", "url": auth_url})
            return

        if parsed.path.startswith("/api/"):
            target = parsed.path[4:]
            if parsed.query:
                target += "?" + parsed.query
            self.proxy_request("GET", target)
            return

        # 정적 파일 서빙
        fname = "chzzk-obs-dock.html" if parsed.path in ("/", "/index.html") else parsed.path.lstrip("/")
        file_path = resource_path(fname)
        if os.path.exists(file_path) and os.path.isfile(file_path):
            with open(file_path, "rb") as f:
                content = f.read()
            ext = os.path.splitext(fname)[1]
            mime = {".html": "text/html", ".css": "text/css", ".js": "application/javascript", ".png": "image/png", ".svg": "image/svg+xml"}.get(ext, "application/octet-stream")
            self.send_response(200)
            self.send_header("Content-Type", mime)
            self.send_header("Content-Length", str(len(content)))
            self.end_headers()
            self.wfile.write(content)
            return

        self.send_error(404)

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        cl = int(self.headers.get("Content-Length", 0))

        if parsed.path == "/save-config":
            data = json.loads(self.rfile.read(cl).decode())
            for k in ("client_id", "client_secret", "redirect_uri", "access_token", "refresh_token"):
                if k in data:
                    config[k] = data[k]
            save_config()
            self.send_json({"status": "ok", "config": config})
            return

        if parsed.path == "/exchange-code":
            data = json.loads(self.rfile.read(cl).decode())
            code, state = data.get("code"), data.get("state", "")
            token_data = exchange_token(code, state)
            if token_data and "accessToken" in token_data:
                config["access_token"] = token_data["accessToken"]
                if "refreshToken" in token_data:
                    config["refresh_token"] = token_data["refreshToken"]
                save_config()
                self.send_json({"status": "ok", "access_token": token_data["accessToken"]})
            else:
                self.send_json({"error": "Token exchange failed"}, 400)
            return

        if parsed.path.startswith("/api/"):
            target = parsed.path[4:]
            if parsed.query:
                target += "?" + parsed.query
            body = self.rfile.read(cl) if cl > 0 else None
            self.proxy_request("POST", target, body)
            return

        self.send_error(404)

    def do_PUT(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path.startswith("/api/"):
            target = parsed.path[4:]
            if parsed.query:
                target += "?" + parsed.query
            cl = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(cl) if cl > 0 else None
            self.proxy_request("PUT", target, body)
            return
        self.send_error(404)

    def do_PATCH(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path.startswith("/api/"):
            target = parsed.path[4:]
            if parsed.query:
                target += "?" + parsed.query
            cl = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(cl) if cl > 0 else None
            self.proxy_request("PATCH", target, body)
            return
        self.send_error(404)


# ============================================================
#  HTML 페이지
# ============================================================
SUCCESS_HTML = """<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="utf-8"><title>치지직 인증 완료</title>
  <style>
    body{background:#0B0E11;color:#00FFA3;font-family:system-ui,-apple-system,sans-serif;text-align:center;padding:70px 20px}
    .card{background:#141921;border:2px solid #00FFA3;border-radius:20px;max-width:440px;margin:0 auto;padding:40px 32px;box-shadow:0 16px 50px rgba(0,255,163,0.25)}
    .icon{font-size:52px;margin-bottom:20px;display:block}
    h2{font-size:24px;font-weight:800;margin-bottom:14px;color:#FFF;letter-spacing:-0.5px}
    p{color:#A8B3C7;font-size:15px;line-height:1.7;margin-bottom:24px}
    .notice{background:rgba(0,255,163,0.08);border:1px solid rgba(0,255,163,0.25);padding:12px;border-radius:10px;margin-bottom:24px;font-size:14px;color:#00FFA3;font-weight:700}
    .btn{background:#00FFA3;color:#0B0E11;font-weight:800;border:none;padding:16px 32px;border-radius:12px;font-size:16px;cursor:pointer;width:100%}
    .btn:hover{background:#00E59B;transform:translateY(-2px);box-shadow:0 6px 20px rgba(0,255,163,0.4)}
    .hint{margin-top:20px;color:#5A6577;font-size:13px}
  </style>
</head>
<body>
  <div class="card">
    <span class="icon">🎉</span>
    <h2>치지직 API 연동 완료!</h2>
    <div class="notice">✅ 토큰 발급 및 저장 성공</div>
    <p>이제 <strong>이 브라우저 창을 닫으셔도 좋습니다.</strong><br>OBS Studio 독 화면에서 방송 관리를 바로 이용하세요!</p>
  </div>
</body>
</html>"""

FAIL_HTML = """<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="utf-8"><title>치지직 인증 실패</title>
  <style>
    body{background:#0B0E11;color:#FF4D6A;font-family:system-ui,sans-serif;text-align:center;padding:60px 20px}
    .card{background:#141921;border:1px solid #FF4D6A;border-radius:16px;max-width:400px;margin:0 auto;padding:32px}
    h2{font-size:20px;margin-bottom:12px}p{color:#A8B3C7;font-size:13px;line-height:1.6}
  </style>
</head>
<body>
  <div class="card">
    <h2>❌ 치지직 토큰 발급 실패</h2>
    <p>Client ID / Client Secret 정보 또는 인가 코드 유효 시간을 확인해 주세요.</p>
  </div>
</body>
</html>"""


# ============================================================
#  시스템 트레이 및 시작 프로그램 제어
# ============================================================
def copy_dock_url(icon=None, item=None):
    url = f"http://localhost:{HTTP_PORT}"
    try:
        import subprocess
        subprocess.run(['clip'], input=url.encode('utf-16le'), check=True)
        if icon:
            icon.notify(f"주소가 클립보드에 복사되었습니다:\n{url}", f"CHZZK Dock {APP_VERSION}")
    except Exception as e:
        print("[Clipboard Error]", e)


def is_startup_enabled():
    try:
        import winreg
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, REG_PATH, 0, winreg.KEY_READ)
        winreg.QueryValueEx(key, APP_NAME)
        winreg.CloseKey(key)
        return True
    except:
        return False


def toggle_startup(icon, item):
    try:
        import winreg
        exe_path = os.path.abspath(sys.argv[0])
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, REG_PATH, 0, winreg.KEY_ALL_ACCESS)
        if is_startup_enabled():
            winreg.DeleteValue(key, APP_NAME)
            icon.notify("시작 프로그램에서 해제되었습니다.", "자동 실행 해제")
        else:
            winreg.SetValueEx(key, APP_NAME, 0, winreg.REG_SZ, f'"{exe_path}"')
            icon.notify("윈도우 시작 프로그램으로 등록되었습니다.", "자동 실행 등록")
        winreg.CloseKey(key)
    except Exception as e:
        print("[Registry Error]", e)


def exit_app(icon, item):
    if icon:
        icon.stop()
    os._exit(0)


def load_tray_icon():
    try:
        from PIL import Image, ImageDraw
        icon_path = resource_path("icon.png")

        if os.path.exists(icon_path):
            return Image.open(icon_path)

        img = Image.new('RGBA', (64, 64), (0, 0, 0, 0))
        d = ImageDraw.Draw(img)
        d.ellipse((8, 8, 56, 56), fill=(0, 255, 163))
        return img
    except Exception as e:
        print("[Tray Icon Error]", e)
        return None


def run_tray():
    try:
        import pystray
        menu = pystray.Menu(
            pystray.MenuItem(f"CHZZK Dock {APP_VERSION}", copy_dock_url, default=True),
            pystray.Menu.SEPARATOR,
            pystray.MenuItem("시작 프로그램 등록", toggle_startup, checked=lambda item: is_startup_enabled()),
            pystray.Menu.SEPARATOR,
            pystray.MenuItem("서버 종료", exit_app)
        )

        icon_img = load_tray_icon()
        if icon_img:
            tray_icon = pystray.Icon(
                "ChzzkDockServer",
                icon_img,
                f"CHZZK OBS Dock Server ({APP_VERSION})",
                menu
            )
            tray_icon.run()
    except Exception as e:
        print("[System Tray Error]", e)


# ============================================================
#  서버 클래스
# ============================================================
class ThreadedServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    allow_reuse_address = False
    daemon_threads = True


# ============================================================
#  메인: HTTP + HTTPS 동시 실행 + 트레이 아이콘
# ============================================================
if __name__ == "__main__":
    # 1) HTTPS 서버 (포트 8080) — 네이버 OAuth 콜백 수신
    https_server = ThreadedServer(("0.0.0.0", HTTPS_PORT), HttpsOAuthHandler)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
    https_server.socket = ctx.wrap_socket(https_server.socket, server_side=True)

    # 2) HTTP 서버 (포트 8081) — OBS 독 위젯 서빙
    http_server = ThreadedServer(("0.0.0.0", HTTP_PORT), HttpDockHandler)

    https_thread = threading.Thread(target=https_server.serve_forever, daemon=True)
    http_thread = threading.Thread(target=http_server.serve_forever, daemon=True)

    print("=" * 56)
    print(f"  CHZZK OBS Dock Server ({APP_VERSION})")
    print(f"  HTTPS (OAuth 콜백) : https://localhost:{HTTPS_PORT}")
    print(f"  HTTP  (OBS 독)     : http://localhost:{HTTP_PORT}")
    print("=" * 56)

    https_thread.start()
    http_thread.start()

    run_tray()
