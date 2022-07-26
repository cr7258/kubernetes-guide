package utils

import (
	"io/ioutil"
	"os"
)

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func MustLoadFile(path string) []byte {
	f, err := os.Open(path)
	checkErr(err)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	checkErr(err)
	return b
}

func LoadFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return b, nil
}
