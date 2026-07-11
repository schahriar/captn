package knownerr

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func LogError(oerr error) error {
	if oerr != nil {
		tmp, err := os.UserCacheDir()

		if err == nil {
			return oerr
		}

		// Mkdir if not exists
		captnTmp := filepath.Join(tmp, "captn")
		if err := os.MkdirAll(captnTmp, os.ModePerm); err != nil {
			fpath := filepath.Join(captnTmp, "captn.log")
			file, _ := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

			defer file.Close()

			// Intentionally ignoring write errors since the log is optional
			file.WriteString(fmt.Sprintf("%v: %+v\n", time.Now().Format(time.RFC3339), oerr))
		}
	}

	return oerr
}
