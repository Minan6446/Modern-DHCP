package parser

type Decoder interface {
	Parse(data []byte) (*Message, error)
}

type WireDecoder struct{}

func NewDecoder() Decoder { return WireDecoder{} }

func (WireDecoder) Parse(data []byte) (*Message, error) { return ParseMessage(data) }
