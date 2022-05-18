package util

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type BaseResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func HttpPost(url, param string) (string, error) {
	return do(http.MethodPost, url, strings.NewReader(param))
}

func HttpGet(url, param string) (string, error) {
	return do(http.MethodGet, url, strings.NewReader(param))
}

func do(method, url string, payload io.Reader) (string, error) {
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("accept-language", "zh-CN")
	req.Header.Set("accept", "*")

	c := http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	baseRes := new(BaseResponse)
	err = json.Unmarshal(body, &baseRes)
	if err != nil {
		return "", err
	}
	if baseRes.Code != 500200 {
		return "", fmt.Errorf("http request failed, failed message: %s", baseRes.Msg)
	}
	data, err := json.Marshal(baseRes.Data)
	if err != nil {
		return "", err
	}
	return string(data), err
}
