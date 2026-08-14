"""
치지직 네이버 로그인 웹뷰 (서브프로세스)
매번 깨끗한 세션으로 로그인 → 쿠키 캡처 → config.json 저장 → 자동 종료
"""
import sys, json, os, time, threading, tempfile, shutil

CONFIG_FILE = "config.json"
LOGIN_HOST = "nid.naver.com"


def get_config_path():
    if getattr(sys, 'frozen', False):
        exe_dir = os.path.dirname(os.path.abspath(sys.executable))
        return os.path.join(exe_dir, CONFIG_FILE)
    script_dir = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(script_dir, CONFIG_FILE)


def save_to_config(nid_aut, nid_ses):
    cpath = get_config_path()
    config = {}
    if os.path.exists(cpath):
        try:
            with open(cpath, "r", encoding="utf-8") as f:
                config = json.load(f)
        except Exception:
            pass
    config["nid_aut"] = nid_aut
    config["nid_ses"] = nid_ses
    with open(cpath, "w", encoding="utf-8") as f:
        json.dump(config, f, ensure_ascii=False, indent=2)


def main():
    try:
        import webview
    except ImportError:
        print("[Webview] pip install pywebview 필요")
        sys.exit(1)

    # 매 로그인 시마다 독립된 임시 프로필 폴더 생성 (이전 세션/쿠키 재사용 방지)
    storage_dir = tempfile.mkdtemp(prefix="chzzk_login_")
    done = threading.Event()

    # 항상 새로운 로그인 폼으로 진입
    login_url = "https://nid.naver.com/nidlogin.login?mode=form"

    win = webview.create_window(
        "치지직 네이버 로그인",
        login_url,
        width=520, height=680, resizable=False,
        on_top=True
    )

    def poll(window):
        time.sleep(2)  # 윈도우 초기화 대기
        for _ in range(300):
            if done.is_set():
                return
            time.sleep(1.5)
            try:
                url = window.evaluate_js("window.location.href") or ""

                # 아직 네이버 로그인 페이지에 머물러 있는 동안에는 절대 닫지 않음
                if not url or LOGIN_HOST in url:
                    continue

                # 사용자가 로그인을 완료하여 외부 도메인으로 리다이렉트된 시점에만 쿠키 수확
                found = {}
                for sc in (window.get_cookies() or []):
                    for name, morsel in sc.items():
                        if name in ("NID_AUT", "NID_SES") and morsel.value:
                            found[name] = morsel.value

                if "NID_AUT" in found and "NID_SES" in found:
                    save_to_config(found["NID_AUT"], found["NID_SES"])
                    print("[Webview] 새로운 네이버 쿠키 저장 완료!")
                    done.set()
                    window.destroy()
                    return
            except Exception:
                pass

    threading.Thread(target=poll, args=(win,), daemon=True).start()

    try:
        # storage_path 지정으로 완벽한 세션 격리 달성
        webview.start(private_mode=True, storage_path=storage_dir)
    finally:
        time.sleep(0.5)
        shutil.rmtree(storage_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
