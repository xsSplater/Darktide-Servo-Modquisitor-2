package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	path := "lang/messages.json"
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	var raw map[string]map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		panic(err)
	}

	// MarshalIndent сортирует ключи мапов (и внешние, и внутренние) по алфавиту.
	out, err := json.MarshalIndent(raw, "", "\t")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		panic(err)
	}
	fmt.Println("messages.json успешно отсортирован и сохранён.")
}
