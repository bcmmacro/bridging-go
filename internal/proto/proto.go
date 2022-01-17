package proto

import (
	"bytes"
	"compress/gzip"
	"encoding/json"

	"github.com/sirupsen/logrus"
)

type Args struct {
	WsID string      `json:"ws_id"`
	Msg  interface{} `json:"msg"`
}

type Packet struct {
	CorrID string `json:"corr_id"`
	Method string `json:"method"`
	Args   Args   `json:"args"`
}

func Deserialize(data []byte) (*Packet, error) {
	buf := bytes.NewBuffer(data)
	r, err := gzip.NewReader(buf)
	if err != nil {
		logrus.Warnf("failed to decompress error[%v]", err)
		return nil, err
	}

	var b bytes.Buffer
	_, err = b.ReadFrom(r)
	if err != nil {
		logrus.Warnf("failed to read from decompressed buffer error[%v]", err)
		return nil, err
	}

	var p Packet
	err = json.Unmarshal(b.Bytes(), &p)
	if err != nil {
		logrus.Warnf("failed to unmarshal json error[%v]", err)
		return nil, err
	}
	return &p, nil
}

func (p *Packet) Serialize(level int) ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		logrus.Warnf("failed to marshal packet error[%v]", err)
		return nil, err
	}

	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		logrus.Warnf("failed to create gzip writer error[%v]", err)
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		logrus.Warnf("failed to compress error[%v]", err)
		return nil, err
	}
	if err = w.Flush(); err != nil {
		logrus.Warnf("failed to flush writer error[%v]", err)
		return nil, err
	}
	if err = w.Close(); err != nil {
		logrus.Warnf("failed to close writer error[%v]", err)
		return nil, err
	}
	return buf.Bytes(), nil
}
