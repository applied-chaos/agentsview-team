package hosts

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed install/linux.sh
var linuxScript string

//go:embed install/uninstall-linux.sh
var uninstallLinuxScript string

//go:embed install/macos.sh
var macosScript string

//go:embed install/macos-uninstall.sh
var macosUninstallScript string

//go:embed install/windows.ps1
var windowsScript string

//go:embed install/windows-uninstall.ps1
var windowsUninstallScript string

// osScript is an embedded install or uninstall script; __PLATFORM_URL__ and __VERSION__ are filled in when served.
type osScript struct{ body, ctype string }

const shellType = "text/x-shellscript; charset=utf-8"

var scripts = map[string]osScript{
	"/install/linux.sh":              {linuxScript, shellType},
	"/uninstall/linux.sh":            {uninstallLinuxScript, shellType},
	"/install/macos.sh":              {macosScript, shellType},
	"/install/macos-uninstall.sh":    {macosUninstallScript, shellType},
	"/install/windows.ps1":           {windowsScript, "text/plain; charset=utf-8"},
	"/install/windows-uninstall.ps1": {windowsUninstallScript, "text/plain; charset=utf-8"},
}

func (s *Service) scriptRoutes(mux *http.ServeMux) {
	for path, sc := range scripts {
		body := strings.NewReplacer("__PLATFORM_URL__", s.baseURL, "__VERSION__", s.version).Replace(sc.body)
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", sc.ctype)
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(body))
		})
	}
}
