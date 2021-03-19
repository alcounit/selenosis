package tools

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

//TimeElapsed ...
func TimeElapsed(t time.Time) string {
	return fmt.Sprintf("%.2fs", time.Since(t).Seconds())
}

//JSONError ...
func JSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(
		map[string]interface{}{
			"value": map[string]string{
				"message": message,
			},
			"code": statusCode,
		},
	)
}

//BuildHostPort ...
func BuildHostPort(session, service, port string) string {
	return net.JoinHostPort(fmt.Sprintf("%s.%s", session, service), port)
}

//StrToFloat64 ...
func StrToFloat64(str string) float64 {
	reg, err := regexp.Compile("[^0-9.]+")
	if err != nil {
		return 0
	}
	s := reg.ReplaceAllString(str, "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
