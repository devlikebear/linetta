; linetta-mcp.exe is spawned by MCP clients (Claude Desktop/Code), not by
; Linetta itself, so the installer's own "close the app" step never stops it
; and the running bridge keeps resources\linetta-mcp.exe locked. Kill it
; before files are written; exit code 128 (no such process) is fine.
!macro NSIS_HOOK_PREINSTALL
  nsExec::ExecToLog 'taskkill /F /T /IM linetta-mcp.exe'
  Pop $0
  Sleep 500
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  nsExec::ExecToLog 'taskkill /F /T /IM linetta-mcp.exe'
  Pop $0
  Sleep 500
!macroend
