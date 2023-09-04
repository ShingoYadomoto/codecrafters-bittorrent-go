package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

// Example:
// - 5:hello -> hello
// - 10:hello12345 -> hello12345
// - i52e -> 52
// - i-52e -> -52
// - l5:helloi52ee -> [“hello”,52]
// - d3:foo3:bar5:helloi52ee -> {"hello": 52, "foo": "bar"}
// - d3:foo10:strawberry5:helloi52ee -> {"foo": "strawberry", "hello": 52}
func decodeBencode(bencodedString string) (interface{}, int, error) {
	if unicode.IsDigit(rune(bencodedString[0])) {
		// string case
		var firstColonIndex int

		for i := 0; i < len(bencodedString); i++ {
			if bencodedString[i] == ':' {
				firstColonIndex = i
				break
			}
		}

		lengthStr := bencodedString[:firstColonIndex]

		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			return "", 0, err
		}

		untilIndex := firstColonIndex + 1 + length
		return bencodedString[firstColonIndex+1 : untilIndex], untilIndex, nil
	} else if strings.HasPrefix(bencodedString, "i") {
		// integers case
		var endIndex int

		for i := 0; i < len(bencodedString); i++ {
			if bencodedString[i] == 'e' {
				endIndex = i
				break
			}
		}

		num, err := strconv.Atoi(bencodedString[1:endIndex])
		if err != nil {
			return "", 0, err
		}

		return num, endIndex + 1, nil
	} else if strings.HasPrefix(bencodedString, "l") {
		// list case
		in := strings.TrimSuffix(strings.TrimPrefix(bencodedString, "l"), "e")

		ret := []interface{}{}
		for {
			decoded, nextIndex, err := decodeBencode(in)
			if err != nil {
				return "", 0, err
			}
			ret = append(ret, decoded)

			in = in[nextIndex:]

			if in == "" {
				break
			}
		}

		return ret, 0, nil
	} else if strings.HasPrefix(bencodedString, "d") {
		// dictionary case
		in := strings.TrimSuffix(strings.TrimPrefix(bencodedString, "d"), "e")

		var (
			ret = map[string]interface{}{}
			key string
		)
		for {
			decoded, nextIndex, err := decodeBencode(in)
			if err != nil {
				return "", 0, err
			}
			if key == "" {
				key = decoded.(string)
			} else {
				ret[key] = decoded
				key = ""
			}

			in = in[nextIndex:]

			if in == "" {
				break
			}
		}

		return ret, 0, nil
	} else {
		return "", 0, fmt.Errorf("only strings, integer are supported at the moment")
	}
}

func main() {
	command := os.Args[1]

	switch command {
	case "decode":
		bencodedValue := os.Args[2]

		decoded, _, err := decodeBencode(bencodedValue)
		if err != nil {
			fmt.Println(err)
			return
		}

		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	case "info":

	default:
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
