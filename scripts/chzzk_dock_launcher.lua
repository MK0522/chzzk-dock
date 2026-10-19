-- ==============================================================================
--  CHZZK OBS Dock Launcher (OBS Studio Lua Script)
--  - OBS 실행 시 chzzk-dock.exe를 자동으로 백그라운드 실행
--  - 독 서버 자동 종료는 chzzk-dock.exe 자체 워치독이 담당
-- ==============================================================================

local obs = obslua
-- 스크립트 버전: 알림 건너뛰기 지원 및 차기 업데이트 감지용 정수 버전(< 비교)
local SCRIPT_VERSION = 3
local BAKED_EXE_PATH = "" -- Go 설치 프로세스에서 실제 chzzk-dock.exe 경로를 자동 주입
local use_default_path = true
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
local shell32 = nil
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
        shell32 = ffi.load("shell32")
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
    return [[
<h3>치지직 OBS 독 자동 실행기</h3>
<p>OBS Studio가 켜질 때 치지직 독을 자동으로 함께 실행합니다.</p>
<p>(OBS를 닫으면 치지직 독도 함께 자동 종료됩니다.)</p>
]]
end

local function get_standard_install_path()
    local lad = os.getenv("LOCALAPPDATA")
    if lad then
        local user_path = lad .. "\\Programs\\CHZZK OBS Dock\\chzzk-dock.exe"
        if file_exists(user_path) then return user_path end
    end
    local pf = os.getenv("ProgramFiles")
    if pf then
        local admin_path = pf .. "\\CHZZK OBS Dock\\chzzk-dock.exe"
        if file_exists(admin_path) then return admin_path end
    end
    local pfx86 = os.getenv("ProgramFiles(x86)")
    if pfx86 then
        local admin_x86 = pfx86 .. "\\CHZZK OBS Dock\\chzzk-dock.exe"
        if file_exists(admin_x86) then return admin_x86 end
    end
    if lad then return lad .. "\\Programs\\CHZZK OBS Dock\\chzzk-dock.exe" end
    return ""
end

local function get_custom_fallback_path()
    if BAKED_EXE_PATH ~= "" and file_exists(BAKED_EXE_PATH) then
        return BAKED_EXE_PATH
    end
    local s_path = script_path()
    if s_path then
        local dir = s_path:match("(.*[/\\])")
        if dir then
            if file_exists(dir .. "chzzk-dock.exe") then return dir .. "chzzk-dock.exe" end
            if file_exists(dir .. "..\\chzzk-dock.exe") then return dir .. "..\\chzzk-dock.exe" end
        end
    end
    if BAKED_EXE_PATH ~= "" then return BAKED_EXE_PATH end
    return get_standard_install_path()
end

local function launch_server_process()
    -- 이미 실행 중이면 불필요한 알림/실행 자체를 생략하고 즉시 종료
    if is_server_already_running() then
        print("[CHZZK Dock] 치지직 독 서버가 이미 백그라운드에서 실행 중입니다. (기동 생략)")
        return
    end

    local target = ""
    if use_default_path then
        target = get_standard_install_path()
    else
        target = (exe_path ~= "" and exe_path or get_custom_fallback_path())
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
    -- [Ponytail/YAGNI] --obs-launch 플래그로 OBS 런처 실행임을 식별하고, OBS 비정상 종료 시 워치독 고아 가드를 활성화합니다.
    local launched = false
    if ffi_loaded and cdef_done then
        local ok, ret = pcall(function()
            local op_w = to_wide("open")
            local file_w = to_wide(target)
            local params_w = to_wide("--obs-launch")
            if op_w and file_w and params_w then
                local lib = shell32 or ffi.C
                local res = lib.ShellExecuteW(nil, op_w, file_w, params_w, nil, 1) -- 1: SW_SHOWNORMAL
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
        local launch_cmd = 'start "" "' .. target .. '" --obs-launch'
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
    use_default_path = obs.obs_data_get_bool(settings, "use_default_path")
    exe_path = obs.obs_data_get_string(settings, "exe_path")
    auto_start = obs.obs_data_get_bool(settings, "auto_start")

    if exe_path == "" then
        exe_path = get_custom_fallback_path()
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
    use_default_path = obs.obs_data_get_bool(settings, "use_default_path")
    exe_path = obs.obs_data_get_string(settings, "exe_path")
    auto_start = obs.obs_data_get_bool(settings, "auto_start")

    if exe_path == "" then
        exe_path = get_custom_fallback_path()
        obs.obs_data_set_string(settings, "exe_path", exe_path)
    end
end

function script_defaults(settings)
    obs.obs_data_set_default_bool(settings, "use_default_path", true)
    obs.obs_data_set_default_string(settings, "exe_path", get_custom_fallback_path())
    obs.obs_data_set_default_bool(settings, "auto_start", true)
end

function script_properties()
    local props = obs.obs_properties_create()

    local p_use_default = obs.obs_properties_add_bool(
        props,
        "use_default_path",
        "기본 경로 사용 (표준 설치 경로 우선 탐색)"
    )

    local p_exe_path = obs.obs_properties_add_path(
        props,
        "exe_path",
        "chzzk-dock.exe 커스텀 경로",
        obs.OBS_PATH_FILE,
        "실행 파일 (*.exe)",
        get_custom_fallback_path()
    )

    -- "기본 경로" 체크 여부에 따라 커스텀 경로 입력 영역 표시/숨김
    obs.obs_property_set_visible(p_exe_path, not use_default_path)

    obs.obs_property_set_modified_callback(p_use_default, function(props, prop, settings)
        local is_def = obs.obs_data_get_bool(settings, "use_default_path")
        local path_prop = obs.obs_properties_get(props, "exe_path")
        if path_prop then
            obs.obs_property_set_visible(path_prop, not is_def)
        end
        return true
    end)

    obs.obs_properties_add_bool(props, "auto_start", "OBS 시작 시 독 서버 자동 실행")

    return props
end

