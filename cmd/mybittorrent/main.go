package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
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
		in := strings.TrimPrefix(bencodedString, "l")

		var (
			ret        = []interface{}{}
			untilIndex int
		)
		for {
			if in[0] == 'e' {
				break
			}

			decoded, nextIndex, err := decodeBencode(in)
			if err != nil {
				return "", 0, err
			}
			ret = append(ret, decoded)

			in = in[nextIndex:]
			untilIndex += nextIndex
		}

		return ret, untilIndex + 1, nil
	} else if strings.HasPrefix(bencodedString, "d") {
		// dictionary case
		in := strings.TrimPrefix(bencodedString, "d")

		var (
			ret        = map[string]interface{}{}
			key        string
			untilIndex int
		)
		for {
			if in[0] == 'e' {
				break
			}

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
			untilIndex += nextIndex
		}

		return ret, untilIndex + 1, nil
	} else {
		return "", 0, fmt.Errorf("unexpected format")
	}
}

func decodeTorrentFile(filepath string) (map[string]interface{}, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	decoded, _, err := decodeBencode(string(content))
	if err != nil {
		return nil, err
	}

	return decoded.(map[string]interface{}), nil
}

func bencode(i interface{}) (string, error) {
	switch i.(type) {
	case string:
		str := i.(string)
		return fmt.Sprintf("%d:%s", len(str), str), nil
	case int:
		num := i.(int)
		return fmt.Sprintf("i%de", num), nil
	case []interface{}:
		joined := ""
		for _, item := range i.([]interface{}) {
			bencoded, err := bencode(item)
			if err != nil {
				return "", err
			}
			joined += bencoded
		}
		return fmt.Sprintf("l%se", joined), nil
	case map[string]interface{}:
		var (
			m    = i.(map[string]interface{})
			keys = make([]string, 0, len(m))
		)
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		joined := ""
		for _, key := range keys {
			bencodedKey, err := bencode(key)
			if err != nil {
				return "", err
			}
			bencodedValue, err := bencode(m[key])
			if err != nil {
				return "", err
			}
			joined = joined + bencodedKey + bencodedValue
		}
		return fmt.Sprintf("d%se", joined), nil
	}

	return "", errors.New("unexpected type")
}

type Info struct {
	TrackerURL  string
	Length      int
	InfoHash    [sha1.Size]byte
	PieceLength int
	PieceHashes string
}

const eachPieceSize = 20

func parseToInfo(torrentFilepath string) (*Info, error) {
	decoded, err := decodeTorrentFile(torrentFilepath)
	if err != nil {
		return nil, err
	}

	metaInfo := decoded["info"].(map[string]interface{})

	info := &Info{
		TrackerURL:  decoded["announce"].(string),
		Length:      metaInfo["length"].(int),
		PieceLength: metaInfo["piece length"].(int),
	}

	bencoded, err := bencode(metaInfo)
	if err != nil {
		return nil, err
	}

	info.InfoHash = sha1.Sum([]byte(bencoded))

	pieceStr := metaInfo["pieces"].(string)
	for i := 0; i < len(pieceStr); i += eachPieceSize {
		info.PieceHashes += fmt.Sprintf("%x\n", pieceStr[i:i+eachPieceSize])
	}

	return info, nil
}

func requestToTracker(torrentFilepath string) (*http.Response, error) {
	info, err := parseToInfo(torrentFilepath)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(info.TrackerURL)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Add("info_hash", string(info.InfoHash[:]))
	q.Add("peer_id", "00112233445566778899")
	q.Add("port", "6881")
	q.Add("uploaded", "0")
	q.Add("downloaded", "0")
	q.Add("left", fmt.Sprint(info.Length))
	q.Add("compact", "1")

	u.RawQuery = q.Encode()

	to := u.String()

	return http.Get(to)
}

func getPeers(torrentFilepath string) ([]string, error) {
	res, err := requestToTracker(torrentFilepath)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	decoded, _, err := decodeBencode(string(b))
	if err != nil {
		return nil, err
	}

	const eachPeerSize = 6

	resPeer := decoded.(map[string]interface{})["peers"].(string)
	if resPeer == "" {
		return nil, errors.New("unexpected peers string")
	}

	ret := make([]string, 0, len(resPeer)/eachPeerSize)
	for i := 0; i < len(resPeer); i += eachPeerSize {
		ip := net.IP(resPeer[i : i+4])
		port := binary.BigEndian.Uint16([]byte(resPeer[i+4 : i+6]))
		ret = append(ret, fmt.Sprintf("%s:%d", ip, port))
	}

	return ret, nil
}

func handshake(conn net.Conn, torrentFilepath string) ([]byte, error) {
	info, err := parseToInfo(torrentFilepath)
	if err != nil {
		return nil, err
	}

	const (
		protocolStrLengthStr = string(byte(19))
		protocolStr          = "BitTorrent protocol"
		reservedBytesStr     = "00000000"
		peerID               = "00112233445566778899"
	)
	infoHash := string(info.InfoHash[:])

	handshake := protocolStrLengthStr + protocolStr + reservedBytesStr + infoHash + peerID
	_, err = conn.Write([]byte(handshake))
	if err != nil {
		return nil, err
	}

	buf := make([]byte, len(handshake))
	_, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[len(handshake)-len(peerID):], nil
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
		torrentFilepath := os.Args[2]

		info, err := parseToInfo(torrentFilepath)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("Tracker URL: %s\n", info.TrackerURL)
		fmt.Printf("Length: %d\n", info.Length)
		fmt.Printf("Info Hash: %x\n", info.InfoHash)
		fmt.Printf("Piece Length: %d\n", info.PieceLength)
		fmt.Printf("Piece Hashes: \n%s", info.PieceHashes)
	case "peers":
		torrentFilepath := os.Args[2]

		peers, err := getPeers(torrentFilepath)
		if err != nil {
			fmt.Println(err)
			return
		}

		for _, peer := range peers {
			fmt.Println(peer)
		}
	case "handshake":
		var (
			torrentFilepath = os.Args[2]
			peer            = os.Args[3]
		)

		conn, err := net.Dial("tcp", peer)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer conn.Close()

		buf, err := handshake(conn, torrentFilepath)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("Peer ID: %x\n", string(buf))
	default:
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}
}
