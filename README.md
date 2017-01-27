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

### translation rules

* the parser will try to translate any function that returns (MimeString, int). MimeString is any string override marked with a comment like `type T string //mimetype a/b`
* if the first argument is an `*http.Request`, it gets passed through to the wrapped function
* if there are any non-Request arguments, the wrapper will try to cast them out of a json request body and crash if it can't. (This is subideal. We should fail more gently and interpolate zero values for missing keys)
* json parsing of the body only happens if necessary -- if your function has 0 args or just an *http.Request arg, the json code isn't emitted in the wrapper
