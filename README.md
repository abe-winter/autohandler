## autohandler

This is a go:generate program that wraps your functions so you can use them in callers expecting HandleFunc.

It expects something like this:

```golang
//go:generate autohandler -pkg main -out wrappers.go
package myserver

import "net/http"

type Json string //mimetype application/json

type Server struct {}

func (self *Server) ApiMethod(name string, flag bool) (Json, int) {
    // ...
    return `{"success":"maybe"}`, http.StatusOK
}
```

and produces something like this:

```golang
package myserver
import (
    "io"
    "net/http"
    "encoding/json"
)
func (self *Server) HandleApiMethod(w http.ResponseWriter, req *http.Request){
    w.Header().Set("Content-Type", "application/json")
    raw := make([]byte, 0, 0)
    if _, err := req.Body.Read(raw); err != nil {panic(err)}
    var parsed map[string]interface{}
    if err := json.Unmarshal(raw, &parsed); err != nil {panic(err)}
    body, retcode := self.TreeDelete(parsed["name"].(string), parsed["flag"].(bool))
    w.WriteHeader(retcode)
    io.WriteString(w, string(body))
}
```

Notes:
* panics at runtime if casting fails.
* should replace missing keys with zero values, but instead will crash
