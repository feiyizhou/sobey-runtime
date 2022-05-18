package src

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type SobeyCniInterface interface {
	InitIpRange() error
	NewSandboxIP() (string, error)
	PutReleasedIP(ip string) error
}

func (ss *sobeyService) InitIpRange() error {
	err := ss.dbService.Put("ipRanges", ss.ipRange)
	if err != nil {
		return err
	}
	return nil
}

func (ss *sobeyService) NewSandboxIP() (string, error) {
	var ip string

	var releasedIPMap map[string]struct{}
	releasedIPStr, err := ss.dbService.Get("releasedIp")
	if err != nil {
		return "", err
	}
	if len(releasedIPStr) != 0 {
		err = json.Unmarshal([]byte(releasedIPStr), &releasedIPMap)
		if err != nil {
			return "", err
		}
		for key := range releasedIPMap {
			ip = key
			break
		}
		delete(releasedIPMap, ip)
		if len(releasedIPMap) == 0 {
			_ = ss.dbService.Delete("releasedIp")
		} else {
			bytes, err := json.Marshal(releasedIPMap)
			if err != nil {
				return "", err
			}
			err = ss.dbService.Put("releasedIp", string(bytes))
			if err != nil {
				return "", err
			}
		}
	} else {
		latestIP, err := ss.dbService.Get("latestIp")
		if err != nil {
			return "", err
		}
		var lastIPBit int
		if len(latestIP) != 0 {
			latestIPArr := strings.Split(latestIP, ".")
			lastIPBit, err = strconv.Atoi(latestIPArr[3])
			if err != nil {
				return "", err
			}
			ip = fmt.Sprintf("%s.%s.%s.%s", latestIPArr[0],
				latestIPArr[1], latestIPArr[2], strconv.Itoa(lastIPBit+1))
		} else {
			ipRanges, err := ss.dbService.Get("ipRanges")
			if err != nil {
				return "", err
			}
			ipRangeArr := strings.Split(strings.Split(ipRanges, "/")[0], ".")
			ip = fmt.Sprintf("%s.%s.%s.%s", ipRangeArr[0],
				ipRangeArr[1], ipRangeArr[2], strconv.Itoa(1))
		}
		err = ss.dbService.Put("latestIp", ip)
		if err != nil {
			return "", err
		}
	}
	return ip, err
}

func (ss *sobeyService) PutReleasedIP(ip string) error {
	var releasedIPMap map[string]struct{}
	releasedIPStr, err := ss.dbService.Get("releasedIp")
	if err != nil {
		return err
	}
	if len(releasedIPStr) != 0 {
		err = json.Unmarshal([]byte(releasedIPStr), &releasedIPMap)
		if err != nil {
			return err
		}
		releasedIPMap[ip] = struct{}{}
	} else {
		releasedIPMap = map[string]struct{}{
			ip: {},
		}
	}
	bytes, err := json.Marshal(releasedIPMap)
	if err != nil {
		return err
	}
	return ss.dbService.Put("releasedIp", string(bytes))
}
