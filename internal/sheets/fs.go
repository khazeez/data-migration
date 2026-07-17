package sheets

import "os"

func readFileFS(path string) ([]byte, error) {
	return os.ReadFile(path)
}
