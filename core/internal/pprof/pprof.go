package pprof

import (
  "net/http"
  _ "net/http/pprof"
)

func PprofStart() {
  http.ListenAndServe("localhost:8080", nil)
}