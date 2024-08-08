package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type AddressBalance struct {
	Address string
	Balance *big.Int
}

func main() {
	start := time.Now()
	token := os.Getenv("TOKEN")
	if token == "" {
		fmt.Println("TOKEN is not set")
		os.Exit(1)
	}

	var err error
	var w sync.WaitGroup
	ch := make(chan string, 10)
	chBlock := make(chan bool, 1)
	const maxBlocks = 100

	blocks := getBlocks(token)
	out := map[string]interface{}{}
	json.Unmarshal([]byte(blocks), &out)
	lastBlock := out["result"].(string)
	addressMap := map[string]*big.Int{}
	for i := 0; i < maxBlocks; i++ {
		w.Add(1)
		if i == 55 {
			time.Sleep(time.Second)
		}
		blockNumber := getBlocksNumber(lastBlock, i)
		go getTransactions(token, blockNumber, ch)
	}
	cnt := 1
	for c := range ch {
		if cnt == maxBlocks {
			close(ch)
		}
		addressMap, err = transactionParser(c, addressMap, &w, chBlock)
		if err != nil {
			fmt.Printf("parse error %s", err)
		}
		cnt++
	}

	w.Wait()
	addressBalance := addressMapToStruct(addressMap)
	cmplx := addressBalance[0].Balance.CmpAbs(addressBalance[len(addressBalance)-1].Balance)
	if cmplx > 0 {
		fmt.Println(addressBalance[0].Address)
	} else if cmplx < 0 {
		fmt.Println(addressBalance[len(addressBalance)-1].Address)
	} else {
		fmt.Println(addressBalance[len(addressBalance)-1].Address)
		fmt.Println(addressBalance[0].Address)
	}

	elapsed := time.Since(start)
	fmt.Printf("%s", elapsed)
}

func addressMapToStruct(addressMap map[string]*big.Int) []AddressBalance {
	var addressBalance []AddressBalance
	for k, v := range addressMap {
		addressBalance = append(addressBalance, AddressBalance{k, v})
	}

	sort.Slice(addressBalance, func(i, j int) bool {
		return addressBalance[i].Balance.Cmp(addressBalance[j].Balance) > 0
	})
	return addressBalance
}

func getBlocksNumber(blockNumber string, k int) string {
	i := new(big.Int)
	fmt.Sscan(blockNumber, i)
	n := i.Sub(i, big.NewInt(int64(k)))
	return fmt.Sprintf("0x%x", n)
}

func transactionParser(
	transactions string,
	addressMap map[string]*big.Int,
	w *sync.WaitGroup,
	ch chan bool,
) (map[string]*big.Int, error) {
	ch <- true
	out := map[string]interface{}{}
	json.Unmarshal([]byte(transactions), &out)
	result, ok := out["result"].(map[string]interface{})
	if !ok {
		return addressMap, fmt.Errorf("result is not map")
	}
	trans, ok := result["transactions"].([]interface{})
	if !ok {
		return addressMap, fmt.Errorf("transactions is not array")
	}
	for _, tr := range trans {
		t, ok := tr.(map[string]interface{})
		if !ok {
			continue
		}
		from, ok := t["from"].(string)
		if !ok {
			continue
		}
		to, ok := t["to"].(string)
		if !ok {
			continue
		}
		val, ok := t["value"].(string)
		if !ok || val == "0x0" {
			continue
		}
		fromHex, ok := addressMap[from]
		if !ok {
			i := new(big.Int)
			fmt.Sscan(val, i)
			i.Sub(big.NewInt(0), i)
			addressMap[from] = i
		} else {
			i := new(big.Int)
			fmt.Sscan(val, i)
			i.Sub(fromHex, i)
			addressMap[from] = i
		}
		toHex, ok := addressMap[to]
		if !ok {
			i := new(big.Int)
			fmt.Sscan(val, i)
			i.Add(big.NewInt(0), i)
			addressMap[to] = i
		} else {
			i := new(big.Int)
			fmt.Sscan(val, i)
			i.Sub(toHex, i)
			addressMap[to] = i
		}
	}
	<-ch
	w.Done()

	return addressMap, nil
}

func getTransactions(token string, blocksNumber string, ch chan string) {
	jsonBody := []byte(fmt.Sprintf(`{"jsonrpc": "2.0","method":"eth_getBlockByNumber","params":["%s",true],"id":"getblock.io"}`, blocksNumber))
	bodyReader := bytes.NewReader(jsonBody)
	res := doRequest(token, bodyReader)
	ch <- res
}

func getBlocks(token string) string {
	jsonBody := []byte(`{"jsonrpc": "2.0","method":  "eth_blockNumber","params":  [],"id":"getblock.io"}`)
	bodyReader := bytes.NewReader(jsonBody)
	return doRequest(token, bodyReader)
}

func doRequest(token string, body io.Reader) string {
	requestURL := fmt.Sprintf("https://go.getblock.io/%s", token)
	req, err := http.NewRequest(http.MethodPost, requestURL, body)
	if err != nil {
		fmt.Printf("client: could not create request: %s\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("client: error making http request: %s\n", err)
		os.Exit(1)
	}

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("client: could not read response body: %s\n", err)
		os.Exit(1)
	}

	return string(resBody)
}
