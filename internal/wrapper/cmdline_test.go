package wrapper

import "testing"

func TestStripProgram(t *testing.T) {
	cases := []struct{ in, want string }{
		{`yt.exe`, ``},
		{`yt.exe -o out.mp4`, `-o out.mp4`},
		{`yt.exe   -o   out.mp4`, `-o   out.mp4`},
		{"yt.exe\t-o out.mp4", `-o out.mp4`},
		{`"C:\Program Files\yt.exe" -o out.mp4`, `-o out.mp4`},
		{`"C:\Program Files\yt.exe"`, ``},
		{`"C:\tools\yt.exe"   --output "%(title)s.%(ext)s"`, `--output "%(title)s.%(ext)s"`},
		{`yt.exe --output "a b" "c  d" -x`, `--output "a b" "c  d" -x`},
		{`yt.exe --re "^\"q\"$"`, `--re "^\"q\"$"`},
		{``, ``},
		{`"unterminated`, ``},
	}
	for _, tc := range cases {
		if got := StripProgram(tc.in); got != tc.want {
			t.Errorf("StripProgram(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
