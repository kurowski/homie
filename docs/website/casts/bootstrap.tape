# Homepage hero — the whole setup story in one command.
#
# A fresh Linux box runs the exact one-liner from homie.sh and ends up fully
# configured. Rendered offline against the nginx sidecar that impersonates
# github.com / raw.githubusercontent.com (see record.sh), so it's the *real*
# bootstrap — just deterministic. hm shows its full TUI because VHS gives it a
# PTY on stdout (hm keys the TUI off stdout, not the curl|bash pipe on stdin).

Output bootstrap.gif
Output bootstrap.webm

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
# Wide enough that the curl one-liner (~104 cols with the prompt) never
# wraps; height trimmed to the ~19 lines the run actually produces.
Set Width 1320
Set Height 600
Set Padding 24
Set TypingSpeed 50ms
Set Framerate 24

# VHS runs bash without rc files and mangles non-ASCII typed input, so source
# the prompt (it carries the ❯) via a pure-ASCII line, then clear. (Hidden.)
Hide
Type "source /etc/homie-ps1.sh" Enter
Type "clear" Enter
Show

Sleep 1s
Type "curl -fsSL https://raw.githubusercontent.com/scouthomes/dotfiles/main/bootstrap.sh | bash"
Sleep 700ms
Enter
Sleep 5s
