#define AppName "ApexVoid Photobooth"
#ifndef AppVersion
#define AppVersion "0.2.1-dev"
#endif
[Setup]
AppId={{48D3E7E4-4F93-4A1E-9C3A-2AFB0C82906D}
AppName={#AppName}
AppVersion={#AppVersion}
DefaultDirName={localappdata}\Programs\ApexVoidPhotobooth
DefaultGroupName={#AppName}
PrivilegesRequired=lowest
OutputDir=..\..\dist
OutputBaseFilename=ApexVoidPhotobooth-setup-unsigned
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\ApexVoidPhotobooth.exe

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; Flags: unchecked

[Files]
Source: "..\..\dist\ApexVoidPhotobooth.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\INSTALL.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\THIRD-PARTY-NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\FFMPEG-LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\NOTO-SANS-OFL.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\ApexVoid Photobooth"; Filename: "{app}\ApexVoidPhotobooth.exe"
Name: "{autodesktop}\ApexVoid Photobooth"; Filename: "{app}\ApexVoidPhotobooth.exe"; Tasks: desktopicon

[Code]
function InitializeSetup(): Boolean;
var Version: String;
    Key: String;
    Found: Boolean;
begin
  Key := 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
  Found := RegQueryStringValue(HKLM32, Key, 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0');
  if not Found then
    Found := RegQueryStringValue(HKCU, Key, 'pv', Version) and (Version <> '') and (Version <> '0.0.0.0');
  if not Found then
  begin
    MsgBox('Microsoft Edge WebView2 Evergreen Runtime is required. Install it from Microsoft, or use the x64 Evergreen Standalone Installer on an offline kiosk, then run this installer again. No download is started automatically.', mbError, MB_OK);
    Result := False;
  end
  else Result := True;
end;
