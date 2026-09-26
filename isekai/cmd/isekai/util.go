package main

import "encoding/json"

func jsonLine(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
