import ctypes
from ctypes import wintypes
import http.server
import json
import os
import subprocess
import sys
import threading
import time
import urllib.parse
import urllib.request
import winreg

# PyInstaller --noconsole 모드에서 sys.stdout, sys.stderr가 None일 때의 충돌 방지
if sys.stdout is None:
    sys.stdout = open(os.devnull, "w", encoding="utf-8")
if sys.stderr is None:
    sys.stderr = open(os.devnull, "w", encoding="utf-8")

# ============================================================
#  CHZZK OBS Dock Server v0.2.1-Beta
#  - 단일 포트 (8081): OBS 브라우저 독 서빙 + 비공식 API 프록시 + 웹뷰 로그인
# ============================================================
APP_VERSION = "v0.2.1-Beta"
APP_NAME = "ChzzkObsDockServer"
REG_PATH = r"Software\Microsoft\Windows\CurrentVersion\Run"
HTTP_PORT = 8081
CONFIG_FILE = "config.json"
USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

# Win32 상수 및 64-bit API 시그니처
user32 = ctypes.windll.user32
shell32 = ctypes.windll.shell32
kernel32 = ctypes.windll.kernel32

LRESULT = ctypes.c_ssize_t
WNDPROCTYPE = ctypes.WINFUNCTYPE(LRESULT, wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM)

class WNDCLASSW(ctypes.Structure):
    _fields_ = [
        ("style", wintypes.UINT),
        ("lpfnWndProc", WNDPROCTYPE),
        ("cbClsExtra", ctypes.c_int),
        ("cbWndExtra", ctypes.c_int),
        ("hInstance", wintypes.HINSTANCE),
        ("hIcon", wintypes.HICON),
        ("hCursor", wintypes.HICON),
        ("hbrBackground", wintypes.HBRUSH),
        ("lpszMenuName", wintypes.LPCWSTR),
        ("lpszClassName", wintypes.LPCWSTR)
    ]

user32.RegisterClassW.argtypes = [ctypes.POINTER(WNDCLASSW)]
user32.RegisterClassW.restype = wintypes.ATOM

user32.CreateWindowExW.argtypes = [
    wintypes.DWORD, wintypes.LPCWSTR, wintypes.LPCWSTR, wintypes.DWORD,
    ctypes.c_int, ctypes.c_int, ctypes.c_int, ctypes.c_int,
    wintypes.HWND, wintypes.HMENU, wintypes.HINSTANCE, wintypes.LPVOID
]
user32.CreateWindowExW.restype = wintypes.HWND

user32.DefWindowProcW.argtypes = [wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM]
user32.DefWindowProcW.restype = LRESULT

user32.LoadImageW.argtypes = [wintypes.HINSTANCE, wintypes.LPCWSTR, wintypes.UINT, ctypes.c_int, ctypes.c_int, wintypes.UINT]
user32.LoadImageW.restype = wintypes.HICON

user32.LoadIconW.argtypes = [wintypes.HINSTANCE, wintypes.LPCWSTR]
user32.LoadIconW.restype = wintypes.HICON

user32.CreatePopupMenu.restype = wintypes.HMENU

user32.AppendMenuW.argtypes = [wintypes.HMENU, wintypes.UINT, ctypes.c_uint64, wintypes.LPCWSTR]
user32.AppendMenuW.restype = wintypes.BOOL

user32.TrackPopupMenu.argtypes = [wintypes.HMENU, wintypes.UINT, ctypes.c_int, ctypes.c_int, ctypes.c_int, wintypes.HWND, ctypes.c_void_p]
user32.TrackPopupMenu.restype = ctypes.c_int

user32.DestroyMenu.argtypes = [wintypes.HMENU]
user32.DestroyMenu.restype = wintypes.BOOL

user32.DestroyWindow.argtypes = [wintypes.HWND]
user32.DestroyWindow.restype = wintypes.BOOL

user32.SetForegroundWindow.argtypes = [wintypes.HWND]
user32.SetForegroundWindow.restype = wintypes.BOOL

user32.GetCursorPos.argtypes = [ctypes.POINTER(wintypes.POINT)]
user32.GetCursorPos.restype = wintypes.BOOL

user32.PostMessageW.argtypes = [wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM]
user32.PostMessageW.restype = wintypes.BOOL

user32.GetMessageW.argtypes = [ctypes.POINTER(wintypes.MSG), wintypes.HWND, wintypes.UINT, wintypes.UINT]
user32.GetMessageW.restype = wintypes.BOOL

user32.TranslateMessage.argtypes = [ctypes.POINTER(wintypes.MSG)]
user32.TranslateMessage.restype = wintypes.BOOL

user32.DispatchMessageW.argtypes = [ctypes.POINTER(wintypes.MSG)]
user32.DispatchMessageW.restype = LRESULT

shell32.Shell_NotifyIconW.argtypes = [wintypes.DWORD, ctypes.c_void_p]
shell32.Shell_NotifyIconW.restype = wintypes.BOOL

WM_USER = 0x0400
WM_TRAYICON = WM_USER + 20
NIM_ADD = 0x00000000
NIM_MODIFY = 0x00000001
NIM_DELETE = 0x00000002
NIF_MESSAGE = 0x00000001
NIF_ICON = 0x00000002
NIF_TIP = 0x00000004
NIF_INFO = 0x00000010
NIIF_INFO = 0x00000001
IMAGE_ICON = 1
LR_LOADFROMFILE = 0x00000010
LR_DEFAULTSIZE = 0x00000040
TPM_RIGHTBUTTON = 0x0002
TPM_RETURNCMD = 0x0100
MF_STRING = 0x0000
MF_SEPARATOR = 0x0800
MF_CHECKED = 0x0008

webview_process = None
tray_instance = None


# ============================================================
#  Config 관리
# ============================================================
def get_config_path():
    if getattr(sys, 'frozen', False):
        exe_dir = os.path.dirname(os.path.abspath(sys.executable))
        return os.path.join(exe_dir, CONFIG_FILE)
    script_dir = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(script_dir, CONFIG_FILE)


def load_config():
    cfg = {"nid_aut": "", "nid_ses": ""}
    cpath = get_config_path()
    if os.path.exists(cpath):
        try:
            with open(cpath, "r", encoding="utf-8") as f:
                cfg.update(json.load(f))
        except Exception:
            pass
    return cfg


def save_config(new_data=None):
    cfg = load_config()
    if new_data:
        cfg.update(new_data)
    cpath = get_config_path()
    try:
        with open(cpath, "w", encoding="utf-8") as f:
            json.dump(cfg, f, ensure_ascii=False, indent=2)
    except Exception:
        pass
    return cfg


# ============================================================
#  HTTP 핸들러 (포트 8081) — OBS 독 위젯 + API 프록시
# ============================================================
class HttpDockHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        try:
            msg = args[0] if args else ""
            if "/config" in msg:
                return
            if sys.stderr and not sys.stderr.closed:
                super().log_message(format, *args)
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

    def send_bytes(self, body, status=200, content_type="application/json"):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
        try:
            self.wfile.flush()
        except Exception:
            pass

    def send_json(self, data, status=200):
        body = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self.send_bytes(body, status=status, content_type="application/json; charset=utf-8")

    def proxy_unofficial_request(self, method, path, body=None, custom_url=None):
        """치지직 비공식 API 프록시 (NID_AUT, NID_SES 쿠키 헤더 포함)"""
        cfg = load_config()
        nid_aut = cfg.get("nid_aut", "")
        nid_ses = cfg.get("nid_ses", "")

        if not nid_aut or not nid_ses:
            self.send_json({"code": 401, "message": "네이버 로그인 쿠키(NID_AUT, NID_SES)가 설정되지 않았습니다. 설정에서 쿠키를 입력하거나 로그인하세요."}, 401)
            return

        if custom_url:
            url = custom_url
        elif path.startswith("/manage/") or path.startswith("/service/"):
            url = f"https://api.chzzk.naver.com{path}"
        else:
            url = f"https://api.chzzk.naver.com/manage/v1{path}"

        headers = {
            "User-Agent": USER_AGENT,
            "Content-Type": "application/json",
            "Cookie": f"NID_AUT={nid_aut}; NID_SES={nid_ses}",
            "Origin": "https://chzzk.naver.com",
            "Referer": "https://chzzk.naver.com/"
        }

        req = urllib.request.Request(url, data=body, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req) as resp:
                self.send_bytes(resp.read(), status=resp.status, content_type=resp.headers.get("Content-Type", "application/json"))
        except urllib.error.HTTPError as e:
            self.send_bytes(e.read(), status=e.code, content_type="application/json")
        except Exception as e:
            self.send_json({"code": 500, "message": str(e)}, status=500)

    def proxy_dispatch(self, method):
        """unofficial API 프록시 디스패처 (DRY 헬퍼)"""
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path.startswith("/unofficial/"):
            target = parsed.path[11:]
            if parsed.query:
                target += "?" + parsed.query
            cl = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(cl) if cl > 0 else None
            self.proxy_unofficial_request(method, target, body)
            return True
        return False

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)

        if parsed.path == "/config":
            self.send_json(load_config())
            return

        if parsed.path == "/login-webview":
            global webview_process
            if webview_process and webview_process.poll() is None:
                self.send_json({"status": "started", "message": "이미 네이버 로그인 창이 열려 있습니다."})
                return

            if getattr(sys, 'frozen', False):
                webview_process = subprocess.Popen([sys.executable, "--login"])
            else:
                base_dir = os.path.dirname(os.path.abspath(__file__))
                script_path = os.path.join(base_dir, "webview_login.py")
                webview_process = subprocess.Popen([sys.executable, script_path])

            self.send_json({"status": "started", "message": "네이버 로그인 웹뷰 창을 생성 중입니다."})
            return

        if parsed.path == "/unofficial-user":
            self.proxy_unofficial_request("GET", "", custom_url="https://comm-api.game.naver.com/nng_main/v1/user/getUserStatus")
            return

        if self.proxy_dispatch("GET"):
            return

        # OBS 독 정적 HTML 서빙
        fname = "chzzk-obs-dock.html" if parsed.path in ("/", "/index.html") else parsed.path.lstrip("/")
        base_dir = getattr(sys, '_MEIPASS', os.path.dirname(os.path.abspath(__file__)))
        fpath = os.path.join(base_dir, fname)
        if not os.path.exists(fpath):
            fpath = os.path.join(os.getcwd(), fname)

        if os.path.exists(fpath) and os.path.isfile(fpath):
            with open(fpath, "rb") as f:
                content = f.read()
            ext = os.path.splitext(fpath)[1]
            mime = {
                ".html": "text/html; charset=utf-8",
                ".css": "text/css; charset=utf-8",
                ".js": "application/javascript; charset=utf-8",
                ".png": "image/png",
                ".svg": "image/svg+xml"
            }.get(ext, "application/octet-stream")
            self.send_bytes(content, status=200, content_type=mime)
            return

        self.send_error(404)

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path in ("/save-config", "/save-cookies"):
            cl = int(self.headers.get("Content-Length", 0))
            data = json.loads(self.rfile.read(cl).decode("utf-8"))
            cfg = save_config({k: data[k] for k in ("nid_aut", "nid_ses") if k in data})
            self.send_json({"status": "ok", "config": cfg})
            return

        if not self.proxy_dispatch("POST"):
            self.send_error(404)

    def do_PUT(self):
        if not self.proxy_dispatch("PUT"):
            self.send_error(404)

    def do_PATCH(self):
        if not self.proxy_dispatch("PATCH"):
            self.send_error(404)


# ============================================================
#  시스템 트레이 (Win32 ctypes 기반 — Zero External Dependencies)
# ============================================================
class NOTIFYICONDATAW(ctypes.Structure):
    _fields_ = [
        ("cbSize", wintypes.DWORD),
        ("hWnd", wintypes.HWND),
        ("uID", wintypes.UINT),
        ("uFlags", wintypes.UINT),
        ("uCallbackMessage", wintypes.UINT),
        ("hIcon", wintypes.HICON),
        ("szTip", wintypes.WCHAR * 128),
        ("dwState", wintypes.DWORD),
        ("dwStateMask", wintypes.DWORD),
        ("szInfo", wintypes.WCHAR * 256),
        ("uTimeoutOrVersion", wintypes.UINT),
        ("szInfoTitle", wintypes.WCHAR * 64),
        ("dwInfoFlags", wintypes.DWORD),
        ("guidItem", ctypes.c_byte * 16),
        ("hBalloonIcon", wintypes.HICON)
    ]


def copy_dock_url():
    url = f"http://localhost:{HTTP_PORT}"
    try:
        subprocess.run(['clip'], input=url.encode('utf-16le'), check=True)
        if tray_instance:
            tray_instance.notify(f"주소가 클립보드에 복사되었습니다:\n{url}", f"CHZZK Dock {APP_VERSION}")
    except Exception as e:
        print("[Clipboard Error]", e)


def is_startup_enabled():
    try:
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, REG_PATH, 0, winreg.KEY_READ)
        winreg.QueryValueEx(key, APP_NAME)
        winreg.CloseKey(key)
        return True
    except Exception:
        return False


def toggle_startup():
    try:
        exe_path = os.path.abspath(sys.argv[0])
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, REG_PATH, 0, winreg.KEY_ALL_ACCESS)
        if is_startup_enabled():
            winreg.DeleteValue(key, APP_NAME)
            if tray_instance:
                tray_instance.notify("시작 프로그램에서 해제되었습니다.", "자동 실행 해제")
        else:
            winreg.SetValueEx(key, APP_NAME, 0, winreg.REG_SZ, f'"{exe_path}"')
            if tray_instance:
                tray_instance.notify("윈도우 시작 프로그램으로 등록되었습니다.", "자동 실행 등록")
        winreg.CloseKey(key)
    except Exception as e:
        print("[Registry Error]", e)


def exit_app():
    if tray_instance:
        tray_instance.stop()
    os._exit(0)


class PureWinTrayIcon:
    def __init__(self, title, icon_path=None):
        self.title = title
        self.icon_path = icon_path
        self.hwnd = None
        self.nid = None
        self.hicon = None
        self.menu_items = []

    def load_icon(self):
        base_dir = getattr(sys, '_MEIPASS', os.path.dirname(os.path.abspath(__file__)))
        candidates = [
            self.icon_path,
            os.path.join(base_dir, "icon.ico"),
            os.path.join(os.getcwd(), "icon.ico"),
            os.path.join(os.path.dirname(sys.executable), "icon.ico")
        ]
        for p in candidates:
            if p and os.path.exists(p):
                self.hicon = user32.LoadImageW(None, os.path.abspath(p), IMAGE_ICON, 0, 0, LR_LOADFROMFILE | LR_DEFAULTSIZE)
                if self.hicon:
                    break

        if not self.hicon:
            hinst = kernel32.GetModuleHandleW(None)
            self.hicon = user32.LoadIconW(hinst, ctypes.c_wchar_p(1))
        if not self.hicon:
            self.hicon = user32.LoadIconW(0, ctypes.c_wchar_p(32512))

    def notify(self, message, title="CHZZK OBS Dock"):
        if not self.nid:
            return
        self.nid.uFlags |= NIF_INFO
        self.nid.szInfo = message[:255]
        self.nid.szInfoTitle = title[:63]
        self.nid.dwInfoFlags = NIIF_INFO
        shell32.Shell_NotifyIconW(NIM_MODIFY, ctypes.byref(self.nid))

    def show_menu(self):
        hmenu = user32.CreatePopupMenu()
        cmd_map = {}
        for idx, item in enumerate(self.menu_items):
            label, callback, is_checked, is_sep = item
            if is_sep:
                user32.AppendMenuW(hmenu, MF_SEPARATOR, 0, None)
            else:
                flags = MF_STRING
                if is_checked and is_checked():
                    flags |= MF_CHECKED
                cmd_id = 1000 + idx
                cmd_map[cmd_id] = callback
                user32.AppendMenuW(hmenu, flags, cmd_id, label)

        pos = wintypes.POINT()
        user32.GetCursorPos(ctypes.byref(pos))
        user32.SetForegroundWindow(self.hwnd)
        selected = user32.TrackPopupMenu(hmenu, TPM_RIGHTBUTTON | TPM_RETURNCMD, pos.x, pos.y, 0, self.hwnd, None)
        user32.PostMessageW(self.hwnd, 0, 0, 0)
        user32.DestroyMenu(hmenu)
        if selected in cmd_map and cmd_map[selected]:
            cmd_map[selected]()

    def run(self):
        self.load_icon()

        def wnd_proc(hwnd, msg, wparam, lparam):
            if msg == WM_TRAYICON:
                if lparam == 0x0205:  # WM_RBUTTONUP (우클릭 메뉴)
                    self.show_menu()
                    return 0
                elif lparam in (0x0202, 0x0203):  # WM_LBUTTONUP / WM_LBUTTONDBLCLK (좌클릭 URL 복사)
                    copy_dock_url()
                    return 0
            elif msg == 0x0010:  # WM_CLOSE
                self.stop()
                return 0
            return user32.DefWindowProcW(hwnd, msg, wparam, lparam)

        self._wnd_proc = WNDPROCTYPE(wnd_proc)

        wcls = WNDCLASSW()
        wcls.lpfnWndProc = self._wnd_proc
        wcls.lpszClassName = "ChzzkDockTrayClass"
        wcls.hInstance = kernel32.GetModuleHandleW(None)
        user32.RegisterClassW(ctypes.byref(wcls))

        self.hwnd = user32.CreateWindowExW(
            0, "ChzzkDockTrayClass", "ChzzkDockTray", 0,
            0, 0, 0, 0, 0, 0, wcls.hInstance, None
        )
        self.nid = NOTIFYICONDATAW()
        self.nid.cbSize = ctypes.sizeof(NOTIFYICONDATAW)
        self.nid.hWnd = self.hwnd
        self.nid.uID = 1
        self.nid.uFlags = NIF_ICON | NIF_MESSAGE | NIF_TIP | NIF_INFO
        self.nid.uCallbackMessage = WM_TRAYICON
        self.nid.hIcon = self.hicon
        self.nid.szTip = self.title[:127]
        # 시작 시 윈도우 알림 센터 정보 메시지
        self.nid.szInfo = f"CHZZK 방송 제어 독 서버가 시작되었습니다.\n(http://localhost:{HTTP_PORT})"
        self.nid.szInfoTitle = "CHZZK OBS Dock"
        self.nid.dwInfoFlags = NIIF_INFO

        shell32.Shell_NotifyIconW(NIM_ADD, ctypes.byref(self.nid))

        msg = wintypes.MSG()
        while user32.GetMessageW(ctypes.byref(msg), 0, 0, 0) > 0:
            user32.TranslateMessage(ctypes.byref(msg))
            user32.DispatchMessageW(ctypes.byref(msg))

    def stop(self):
        if self.nid:
            shell32.Shell_NotifyIconW(NIM_DELETE, ctypes.byref(self.nid))
            self.nid = None
        if self.hwnd:
            user32.DestroyWindow(self.hwnd)
            self.hwnd = None
        os._exit(0)


def run_tray():
    global tray_instance
    try:
        base_path = getattr(sys, '_MEIPASS', os.path.dirname(os.path.abspath(__file__)))
        icon_path = os.path.join(base_path, "icon.ico")

        tray = PureWinTrayIcon(f"CHZZK OBS Dock Server ({APP_VERSION})", icon_path)
        tray.menu_items = [
            (f"CHZZK Dock {APP_VERSION}", copy_dock_url, None, False),
            (None, None, None, True),
            ("시작 프로그램 등록", toggle_startup, is_startup_enabled, False),
            (None, None, None, True),
            ("서버 종료", exit_app, None, False)
        ]
        tray_instance = tray
        tray.run()
    except Exception as e:
        print("[System Tray Error]", e)
        while True:
            time.sleep(1)


class ExclusiveThreadingServer(http.server.ThreadingHTTPServer):
    allow_reuse_address = False
    daemon_threads = True


# ============================================================
#  메인: HTTP 서버 실행 + 트레이 아이콘
# ============================================================
if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] in ("--login", "webview_login.py", "-l"):
        import webview_login
        webview_login.main()
        sys.exit(0)

    try:
        http_server = ExclusiveThreadingServer(("127.0.0.1", HTTP_PORT), HttpDockHandler)
    except OSError as e:
        if getattr(e, 'winerror', None) == 10048 or e.errno == 10048 or "10048" in str(e) or "address already in use" in str(e).lower():
            msg = f"CHZZK Dock 서버가 이미 실행 중이거나 포트({HTTP_PORT})가 사용 중입니다.\n\n작업 표시줄 트레이 아이콘이나 기존 실행 중인 프로그램을 확인해 주세요."
            print(f"\n[오류] {msg}\n")
            try:
                ctypes.windll.user32.MessageBoxW(0, msg, "CHZZK OBS Dock - 실행 오류", 0x10 | 0x0)
            except Exception:
                pass
            sys.exit(1)
        raise

    print("=" * 60)
    print(f"  CHZZK OBS Dock Server ({APP_VERSION})")
    print(f"  HTTP  (통합 방송 독) : http://localhost:{HTTP_PORT}")
    print("=" * 60)

    http_thread = threading.Thread(target=http_server.serve_forever, daemon=True)
    http_thread.start()

    run_tray()
