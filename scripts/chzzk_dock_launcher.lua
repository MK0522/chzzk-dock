-- ==============================================================================
--  CHZZK OBS Dock Launcher (OBS Studio Lua Script)
--  - OBS 실행 시 chzzk-dock.exe를 자동으로 백그라운드 실행
--  - 독 서버 자동 종료는 chzzk-dock.exe 자체 워치독이 담당
-- ==============================================================================

local obs = obslua
-- 스크립트 버전: 알림 건너뛰기 지원 및 차기 업데이트 감지용 정수 버전(< 비교)
local SCRIPT_VERSION = 2
local BAKED_EXE_PATH = "" -- Go 설치 프로세스에서 실제 chzzk-dock.exe 경로를 자동 주입
local exe_path = ""
local auto_start = true

local function file_exists(path)
    if not path or path == "" then return false end
    local f = io.open(path, "r")
    if f ~= nil then
        io.close(f)
        return true
    end
    return false
end

local ffi_loaded, ffi = pcall(require, "ffi")
local cdef_done = false
if ffi_loaded then
    pcall(function()
        ffi.cdef[[
            int MultiByteToWideChar(unsigned int CodePage, unsigned long dwFlags, const char* lpMultiByteStr, int cbMultiByte, uint16_t* lpWideCharStr, int cchWideChar);
            int MessageBoxW(void* hWnd, const uint16_t* lpText, const uint16_t* lpCaption, unsigned int uType);
            void* OpenMutexW(unsigned long dwDesiredAccess, int bInheritHandle, const uint16_t* lpName);
            int CloseHandle(void* hObject);
            void* ShellExecuteW(void* hwnd, const uint16_t* lpOperation, const uint16_t* lpFile, const uint16_t* lpParameters, const uint16_t* lpDirectory, int nShowCmd);
        ]]
        cdef_done = true
    end)
end

local function to_wide(str)
    if not ffi_loaded or not str then return nil end
    local CP_UTF8 = 65001
    local len = ffi.C.MultiByteToWideChar(CP_UTF8, 0, str, -1, nil, 0)
    if len <= 0 then return nil end
    local buf = ffi.new("uint16_t[?]", len)
    ffi.C.MultiByteToWideChar(CP_UTF8, 0, str, -1, buf, len)
    return buf
end

local function alert_error(msg)
    if ffi_loaded and cdef_done then
        pcall(function()
            local title_w = to_wide("CHZZK OBS Dock 알림")
            local text_w = to_wide(msg)
            if title_w and text_w then
                ffi.C.MessageBoxW(nil, text_w, title_w, 0x10) -- 0x10: MB_ICONERROR
            end
        end)
    end
    print("[CHZZK Dock 오류] " .. msg)
end

local function is_server_already_running()
    if ffi_loaded and cdef_done then
        local ok, running = pcall(function()
            local SYNCHRONIZE = 0x00100000
            local name_w = to_wide("Local\\ChzzkDock")
            if not name_w then return false end
            local h = ffi.C.OpenMutexW(SYNCHRONIZE, 0, name_w)
            if h ~= nil and h ~= ffi.null then
                ffi.C.CloseHandle(h)
                return true
            end
            return false
        end)
        if ok and running then return true end
    end
    return false
end

function script_description()
    local cur_path = script_path() or "(알 수 없음)"
    return string.format([[
<h2>🎮 치지직 OBS 독 자동 실행기 (CHZZK Dock Launcher)</h2>
<p>OBS Studio가 켜질 때 <b>치지직 독 서버(chzzk-dock.exe)</b>를 자동으로 백그라운드 실행합니다.</p>
<p><i>(서버 종료는 독 서버 자체에 내장된 OBS 감지 워치독이 안전하게 전담합니다.)</i></p>
<hr/>
<p><b>📌 사용법:</b></p>
<ol>
  <li>최초 1회 등록 시 아래 [chzzk-dock.exe 경로]가 자동으로 설정됩니다.</li>
  <li>평소처럼 OBS만 켜시면 독 서버가 100%%%% 자동 실행됩니다.</li>
</ol>
<p style="color: #ffaa00;">⚠️ <b>주의사항:</b> 독 서버 실행 파일의 이름이 반드시 <code>chzzk-dock.exe</code>여야 정상 감지됩니다. 파일명을 임의로 변경하지 마세요.</p>
<hr/>
<p><b>🗑️ 스크립트 삭제/제거 방법:</b></p>
<ol>
  <li><b>[필수]</b> OBS 상단 메뉴 [도구] ➔ [스크립트] 창 좌측 목록에서 <code>chzzk_dock_launcher.lua</code>를 선택하고 <b>[-]</b> 버튼을 먼저 눌러 등록을 해제하세요.</li>
  <li>등록 해제 후 아래 경로에 위치한 스크립트 파일을 삭제하시면 됩니다:</li>
</ol>
<p><code>%s</code></p>
]], cur_path)
end

local function get_default_exe_path()
    if BAKED_EXE_PATH ~= "" and file_exists(BAKED_EXE_PATH) then
        return BAKED_EXE_PATH
    end

    local candidates = {}
    local s_path = script_path()
    if s_path then
        local dir = s_path:match("(.*[/\\])")
        if dir then
            table.insert(candidates, dir .. "chzzk-dock.exe")
            table.insert(candidates, dir .. "..\\chzzk-dock.exe")
        end
    end

    local lad = os.getenv("LOCALAPPDATA")
    if lad then table.insert(candidates, lad .. "\\Programs\\CHZZK OBS Dock\\chzzk-dock.exe") end
    local pf = os.getenv("ProgramFiles")
    if pf then table.insert(candidates, pf .. "\\CHZZK OBS Dock\\chzzk-dock.exe") end
    local pfx86 = os.getenv("ProgramFiles(x86)")
    if pfx86 then table.insert(candidates, pfx86 .. "\\CHZZK OBS Dock\\chzzk-dock.exe") end

    for _, path in ipairs(candidates) do
        if file_exists(path) then
            return path
        end
    end

    if BAKED_EXE_PATH ~= "" then return BAKED_EXE_PATH end
    if lad then return lad .. "\\Programs\\CHZZK OBS Dock\\chzzk-dock.exe" end
    return ""
end

local function launch_server_process()
    -- 이미 실행 중이면 불필요한 알림/실행 자체를 생략하고 즉시 종료
    if is_server_already_running() then
        print("[CHZZK Dock] 치지직 독 서버가 이미 백그라운드에서 실행 중입니다. (기동 생략)")
        return
    end

    local target = exe_path
    if target == "" then
        target = get_default_exe_path()
    end

    if target == "" or not file_exists(target) then
        local display_target = (target ~= "" and target or "(경로 비어있음)")
        local err_msg = "치지직 독 서버를 실행하지 못했습니다.\n\n" ..
            "지정된 경로에 chzzk-dock.exe 파일이 존재하지 않습니다:\n" ..
            display_target .. "\n\n" ..
            "OBS 상단 메뉴 [도구] -> [스크립트]의 chzzk_dock_launcher.lua 설정에서 chzzk-dock.exe 파일의 정확한 경로를 지정해 주세요."
        alert_error(err_msg)
        return
    end

    print("[CHZZK Dock] 치지직 독 서버 시작 요청: " .. target)
    -- cmd.exe 콘솔 경합 및 표준 I/O 핸들 상속으로 인한 OBS 데드락을 원천 방지하기 위해 Win32 ShellExecuteW를 직접 호출합니다.
    local launched = false
    if ffi_loaded and cdef_done then
        local ok, ret = pcall(function()
            local op_w = to_wide("open")
            local file_w = to_wide(target)
            if op_w and file_w then
                local res = ffi.C.ShellExecuteW(nil, op_w, file_w, nil, nil, 1) -- 1: SW_SHOWNORMAL
                -- ShellExecuteW 반환값이 32보다 크면 성공
                if tonumber(ffi.cast("intptr_t", res)) > 32 then
                    return true
                end
            end
            return false
        end)
        if ok and ret then
            launched = true
        end
    end

    if not launched then
        -- FFI 호출 실패 시 폴백
        local launch_cmd = 'start "" "' .. target .. '"'
        local ret = os.execute(launch_cmd)
        if ret ~= 0 then
            alert_error("치지직 독 서버를 실행하지 못했습니다.\n\n명령: " .. launch_cmd)
        end
    end
end

local has_started = false

local function on_frontend_event(event)
    -- OBS의 장면/소스/UI 윈도우 초기화가 완전히 완료된 시점에 딱 1회 안전하게 독 서버 기동
    if event == obs.OBS_FRONTEND_EVENT_FINISHED_LOADING then
        if not has_started then
            has_started = true
            if auto_start then
                launch_server_process()
            end
        end
    end
end

function script_load(settings)
    if exe_path == "" then
        exe_path = get_default_exe_path()
    end

    -- OBS UI 렌더링 및 메인 윈도우 생성 완료 이벤트를 감지하여 안전하게 비동기 기동합니다.
    obs.obs_frontend_add_event_callback(on_frontend_event)
end

function script_unload()
    -- 프론트엔드 이벤트 콜백 해제
    obs.obs_frontend_remove_event_callback(on_frontend_event)
    -- 독 서버의 자동 종료는 백엔드 자체 워치독이 담당하므로 별도의 강제 종료(taskkill)를 수행하지 않습니다.
end

function script_update(settings)
    exe_path = obs.obs_data_get_string(settings, "exe_path")
    auto_start = obs.obs_data_get_bool(settings, "auto_start")

    if exe_path == "" then
        exe_path = get_default_exe_path()
        obs.obs_data_set_string(settings, "exe_path", exe_path)
    end
end

function script_defaults(settings)
    obs.obs_data_set_default_string(settings, "exe_path", get_default_exe_path())
    obs.obs_data_set_default_bool(settings, "auto_start", true)
end

function script_properties()
    local props = obs.obs_properties_create()

    obs.obs_properties_add_path(
        props,
        "exe_path",
        "chzzk-dock.exe 경로",
        obs.OBS_PATH_FILE,
        "실행 파일 (*.exe)",
        get_default_exe_path()
    )

    obs.obs_properties_add_bool(props, "auto_start", "OBS 시작 시 독 서버 자동 실행")

    return props
end

