@echo off
REM AppBip - chay tren Windows (Samsung tablet + ADB)
REM Sua duong dan adb.exe neu can:

if "%APPBIP_ADB%"=="" (
  if exist "C:\platform-tools\adb.exe" set "APPBIP_ADB=C:\platform-tools\adb.exe"
  if exist "%LOCALAPPDATA%\Android\Sdk\platform-tools\adb.exe" set "APPBIP_ADB=%LOCALAPPDATA%\Android\Sdk\platform-tools\adb.exe"
)

if "%APPBIP_ADB%"=="" (
  echo ERROR: Khong tim thay adb.exe
  echo Tai Platform Tools: https://developer.android.com/tools/releases/platform-tools
  echo Giai nen vao C:\platform-tools roi chay lai file nay.
  echo Hoac set: set APPBIP_ADB=C:\duong\dan\adb.exe
  exit /b 1
)

echo Using ADB: %APPBIP_ADB%
"%APPBIP_ADB%" devices
echo.

if "%1"=="" (
  echo Usage:
  echo   run.bat check-in
  echo   run.bat check-out
  echo   run.bat schedule
  exit /b 1
)

"%~dp0appbip.exe" %*
