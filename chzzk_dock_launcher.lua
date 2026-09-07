-- ==============================================================================
--  CHZZK OBS Dock Launcher (OBS Studio Lua Script)
--  - OBS 실행 시 chzzk-dock.exe를 자동으로 백그라운드 실행
--  - OBS 종료 시 chzzk-dock.exe를 자동으로 안전하게 종료
-- ==============================================================================

local obs = obslua
local BAKED_EXE_PATH = "" -- Go 설치 프로세스에서 실제 chzzk-dock.exe 경로를 자동 주입
local exe_path = ""
local auto_start = true
local auto_stop = true

local function file_exists(path)
    if not path or path == "" then return false end
    local f = io.open(path, "r")
    if f ~= nil then
        io.close(f)
        return true
    end
    return false
end

local function alert_error(msg)
    -- OBS LuaJIT FFI를 통해 Windows 네이티브 MessageBox 호출
    local has_ffi, ffi = pcall(require, "ffi")
    if has_ffi then
        pcall(function()
            ffi.cdef[[
                int MessageBoxA(void* hWnd, const char* lpText, const char* lpCaption, unsigned int uType);
            ]]
            ffi.C.MessageBoxA(nil, msg, "CHZZK OBS Dock 알림", 0x10) -- 0x10: MB_ICONERROR
        end)
    end
    print("[CHZZK Dock 오류] " .. msg)
end

function script_description()
    return [[
<h2>🎮 치지직 OBS 독 자동 실행기 (CHZZK Dock Launcher)</h2>
<p>OBS Studio가 켜질 때 <b>치지직 독 서버(chzzk-dock.exe)</b>를 자동으로 실행하고, OBS가 꺼질 때 자동으로 종료합니다.</p>
<hr/>
<p><b>사용법 및 상태:</b></p>
<ol>
  <li>아래 [실행 파일 경로]가 올바른지 확인하세요. (최초 설치 시 자동 입력됩니다.)</li>
  <li>정상 설정되면 평소처럼 OBS만 켜고 끄시면 독 서버가 100% 자동 동작합니다!</li>
</ol>
]]
end

local function get_default_exe_path()
    if BAKED_EXE_PATH ~= "" and file_exists(BAKED_EXE_PATH) then
        return BAKED_EXE_PATH
    end
    local s_path = script_path()
    if s_path then
        local dir = s_path:match("(.*[/\\])")
        if dir and file_exists(dir .. "chzzk-dock.exe") then
            return dir .. "chzzk-dock.exe"
        end
    end
    if BAKED_EXE_PATH ~= "" then
        return BAKED_EXE_PATH
    end
    return ""
end

local function start_dock_server()
    local target = exe_path
    if target == "" then
        target = get_default_exe_path()
    end

    if target == "" or not file_exists(target) then
        local err_msg = "치지직 독 실행 파일(chzzk-dock.exe)을 찾을 수 없습니다.\n\n지정된 경로:\n" .. (target ~= "" and target or "(경로 비어있음)") .. "\n\n파일이 이동되었거나 삭제되었는지 확인해 주세요.\nOBS 상단 메뉴 [도구] -> [스크립트]에서 올바른 chzzk-dock.exe 위치를 지정하세요."
        alert_error(err_msg)
        return
    end

    -- 이미 실행 중인지 확인
    local check_cmd = 'tasklist /fi "imagename eq chzzk-dock.exe" | findstr /i "chzzk-dock.exe" >nul'
    local is_running = (os.execute(check_cmd) == 0)

    if not is_running then
        print("[CHZZK Dock] 치지직 독 서버 시작 중: " .. target)
        -- Windows 창 숨김 백그라운드 실행
        local launch_cmd = 'start "" /b "' .. target .. '"'
        local ret = os.execute(launch_cmd)
        if ret ~= 0 then
            alert_error("치지직 독 서버 실행에 실패했습니다.\n\n명령: " .. launch_cmd)
        end
    else
        print("[CHZZK Dock] 치지직 독 서버가 이미 실행 중입니다.")
    end
end

local function stop_dock_server()
    print("[CHZZK Dock] 치지직 독 서버 종료 중...")
    os.execute('taskkill /f /im chzzk-dock.exe >nul 2>&1')
end

function script_load(settings)
    if exe_path == "" then
        exe_path = get_default_exe_path()
    end

    if auto_start then
        start_dock_server()
    end
end

function script_unload()
    if auto_stop then
        stop_dock_server()
    end
end

function script_update(settings)
    exe_path = obs.obs_data_get_string(settings, "exe_path")
    auto_start = obs.obs_data_get_bool(settings, "auto_start")
    auto_stop = obs.obs_data_get_bool(settings, "auto_stop")

    if exe_path == "" then
        exe_path = get_default_exe_path()
        obs.obs_data_set_string(settings, "exe_path", exe_path)
    end
end

function script_defaults(settings)
    obs.obs_data_set_default_string(settings, "exe_path", get_default_exe_path())
    obs.obs_data_set_default_bool(settings, "auto_start", true)
    obs.obs_data_set_default_bool(settings, "auto_stop", true)
end

function btn_start_clicked(props, prop)
    start_dock_server()
    return true
end

function btn_stop_clicked(props, prop)
    stop_dock_server()
    return true
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
    obs.obs_properties_add_bool(props, "auto_stop", "OBS 종료 시 독 서버 자동 종료")

    obs.obs_properties_add_button(props, "btn_start", "▶️ 지금 서버 켜기", btn_start_clicked)
    obs.obs_properties_add_button(props, "btn_stop", "⏹️ 지금 서버 끄기", btn_stop_clicked)

    return props
end

