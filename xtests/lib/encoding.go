package main

import "encoding"

type TextMarshaled struct {
	text string
}

func (t TextMarshaled) MarshalText() ([]byte, error) {
	return []byte(t.text), nil
}

func (t *TextMarshaled) UnmarshalText(text []byte) error {
	t.text = string(text)
	return nil
}

func (t TextMarshaled) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte(t.text)...), nil
}

type BinaryMarshaled struct {
	data string
}

func (b BinaryMarshaled) MarshalBinary() ([]byte, error) {
	return []byte(b.data), nil
}

func (b *BinaryMarshaled) UnmarshalBinary(data []byte) error {
	b.data = string(data)
	return nil
}

func (b BinaryMarshaled) AppendBinary(d []byte) ([]byte, error) {
	return append(d, []byte(b.data)...), nil
}

type BothMarshaled struct {
	text string
}

func (t BothMarshaled) MarshalText() ([]byte, error) {
	return []byte(t.text), nil
}

func (t BothMarshaled) MarshalBinary() ([]byte, error) {
	return []byte(t.text), nil
}

func TestTextMarshaler() int {
	var m encoding.TextMarshaler
	v := TextMarshaled{text: "hello"}
	m = v
	data, err := m.MarshalText()
	if err != nil {
		return 1
	}
	if string(data) != "hello" {
		return 2
	}
	return 0
}

func TestTextUnmarshaler() int {
	var u encoding.TextUnmarshaler
	v := &TextMarshaled{}
	u = v
	err := u.UnmarshalText([]byte("world"))
	if err != nil {
		return 1
	}
	if v.text != "world" {
		return 2
	}
	return 0
}

func TestTextAppender() int {
	var a encoding.TextAppender
	v := TextMarshaled{text: "hello"}
	a = v
	data, err := a.AppendText([]byte("pre:"))
	if err != nil {
		return 1
	}
	if string(data) != "pre:hello" {
		return 2
	}
	return 0
}

func TestBinaryMarshaler() int {
	var m encoding.BinaryMarshaler
	v := BinaryMarshaled{data: "bin"}
	m = v
	data, err := m.MarshalBinary()
	if err != nil {
		return 1
	}
	if string(data) != "bin" {
		return 2
	}
	return 0
}

func TestBinaryUnmarshaler() int {
	var u encoding.BinaryUnmarshaler
	v := &BinaryMarshaled{}
	u = v
	err := u.UnmarshalBinary([]byte("data"))
	if err != nil {
		return 1
	}
	if v.data != "data" {
		return 2
	}
	return 0
}

func TestBinaryAppender() int {
	var a encoding.BinaryAppender
	v := BinaryMarshaled{data: "bin"}
	a = v
	data, err := a.AppendBinary([]byte("pre:"))
	if err != nil {
		return 1
	}
	if string(data) != "pre:bin" {
		return 2
	}
	return 0
}

func TestBothInterfaces() int {
	var tm encoding.TextMarshaler
	var bm encoding.BinaryMarshaler
	v := BothMarshaled{text: "both"}
	tm = v
	bm = v
	data, err := tm.MarshalText()
	if err != nil {
		return 1
	}
	if string(data) != "both" {
		return 2
	}
	data2, err2 := bm.MarshalBinary()
	if err2 != nil {
		return 3
	}
	if string(data2) != "both" {
		return 4
	}
	return 0
}

func TestNilMethodValue() int {
	var u encoding.TextUnmarshaler
	v := &TextMarshaled{}
	u = v
	if u.UnmarshalText([]byte("")) != nil {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestTextMarshaler", TestTextMarshaler)
	runTest("TestTextUnmarshaler", TestTextUnmarshaler)
	runTest("TestTextAppender", TestTextAppender)
	runTest("TestBinaryMarshaler", TestBinaryMarshaler)
	runTest("TestBinaryUnmarshaler", TestBinaryUnmarshaler)
	runTest("TestBinaryAppender", TestBinaryAppender)
	runTest("TestBothInterfaces", TestBothInterfaces)
	runTest("TestNilMethodValue", TestNilMethodValue)
}
