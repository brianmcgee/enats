package web3

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/juju/errors"
)

const (
	MethodEthSubscription = "eth_subscription"

	clientVersionPattern = "^(.*?)/(.*?)(/(.*?)/(.*?))?$"
)

func ParseClientVersion(str string) (ClientVersion, error) {
	var result ClientVersion

	regex, err := regexp.Compile(clientVersionPattern)
	if err != nil {
		panic("Failed to compile rpc version pattern")
	}

	matches := regex.FindStringSubmatch(str)

	if len(matches) < 2 {
		return result, errors.Errorf("Expected at least 2 matches, found %d. Input = %s", len(matches), str)
	}

	result = ClientVersion{
		Name:    matches[1],
		Version: matches[2],
	}

	if len(matches) == 6 {
		result.OS = matches[4]
		result.Language = matches[5]
	}

	return result, nil
}

type NodeInfo struct {
	Id            string          `json:"id"`
	Ip            string          `json:"ip"`
	Name          string          `json:"name"`
	Enode         string          `json:"enode"`
	Ports         json.RawMessage `json:"ports"`
	Protocols     json.RawMessage `json:"protocols"`
	ListenAddress string          `json:"listenAddr"`
}

func (n *NodeInfo) ParseClientVersion() (ClientVersion, error) {
	return ParseClientVersion(n.Name)
}

type ClientVersion struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Language string `json:"language"`
}

func (cv ClientVersion) String() string {
	if cv.OS == "" || cv.Language == "" {
		return fmt.Sprintf("%s/%s", cv.Name, cv.Version)
	} else {
		return fmt.Sprintf("%s/%s/%s/%s", cv.Name, cv.Version, cv.OS, cv.Language)
	}
}

type BlockHeader struct {
	Hash       string `json:"hash"`
	Number     string `json:"number"`
	ParentHash string `json:"parentHash"`
	Timestamp  string `json:"timestamp"`
}

func (h *BlockHeader) MustNumberUint64() uint64 {
	return hexutil.MustDecodeUint64(h.Number)
}

func (h *BlockHeader) NumberUint64() (uint64, error) {
	return hexutil.DecodeUint64(h.Number)
}

type ClientInfo struct {
	NetworkId     uint64
	ChainId       uint64
	ClientId      string        `json:"clientId"`
	ClientVersion ClientVersion `json:"clientVersion"`
}

type SubscriptionNotification struct {
	SubscriptionId string          `json:"subscription"`
	Result         json.RawMessage `json:"result"`
}

func (sn *SubscriptionNotification) UnmarshalResult(result interface{}) error {
	return json.Unmarshal(sn.Result, &result)
}
