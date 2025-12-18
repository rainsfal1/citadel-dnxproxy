package logging

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

type LogItem struct {
	HostToResolve string
	RequestedBy   string
	Action        string
	Duration      float64
	UserID        string
	UserName      string
	BlockReason   string
}

type LogClient interface {
	Connect() error
	Close() error
	WriteItem(item *LogItem) error
}

type LogFileClient struct {
	filename string
	file     *os.File
	Sink     io.Writer
}

// ErrFileNotOpen raised when the log file is not opened
var ErrFileNotOpen = errors.New("File not open, did you check return from New??")

func NewLogFileClient(FileName string) (*LogFileClient, error) {
	client := LogFileClient{
		filename: FileName,
	}

	if FileName == "-" {
		client.file = os.Stdout
		client.Sink = os.Stdout
	} else {
		f, err := os.OpenFile(FileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}

		client.file = f
		client.Sink = f

	}

	return &client, nil
}

// Not implemented/needed
func (c *LogFileClient) Connect() error {
	return nil
}

func (c *LogFileClient) Close() error {
	if c.file == nil {
		return ErrFileNotOpen
	}
	return c.file.Close()
}

func (c *LogFileClient) WriteItem(item *LogItem) error {
	if c.file == nil {
		return ErrFileNotOpen
	}
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
	strItem := fmt.Sprintf("%s,%s,%s,%s,%f,%s,%s,%s\n",
		ts,
		item.HostToResolve,
		item.RequestedBy,
		item.Action,
		item.Duration,
		item.UserID,
		item.UserName,
		item.BlockReason)

	buf := bytes.NewBufferString(strItem)
	_, err := c.Sink.Write(buf.Bytes())
	return err

}
