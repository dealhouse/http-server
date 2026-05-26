package httpserver

import (
	// "log"
	"fmt"
	"net/http"
)

func main() {
	sm := http.NewServeMux()
	server := http.Server{
		Handler: sm,
		Addr:    ".8080",
	}
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println("listen and serve error")
		return
	}
}
