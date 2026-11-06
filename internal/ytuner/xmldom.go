package ytuner

// A tiny DOM that reproduces the byte layout of FPC's XMLWrite/XMLRead, which
// some AVRs may depend on:
//   <?xml version="1.0" encoding="utf-8"?>
//   <ListOfItems>
//     <ItemCount>1</ItemCount>      (2 space indentation, platform line ending)
//     <Empty></Empty>               (element with an empty text node)
//     <Empty/>                      (element without children, e.g. after reading a file)
//   </ListOfItems>

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strings"
)

// XNode is an element node with either element children or a single text child.
type XNode struct {
	Name     string
	HasText  bool
	Text     string
	Children []*XNode
}

// NewElem creates an element.
func NewElem(name string) *XNode { return &XNode{Name: name} }

// NewTextElem creates an element with a text child (possibly empty).
func NewTextElem(name, text string) *XNode { return &XNode{Name: name, HasText: true, Text: text} }

// Append adds a child and returns it.
func (n *XNode) Append(c *XNode) *XNode {
	n.Children = append(n.Children, c)
	return c
}

// Find mirrors TDOMNode.FindNode (first direct child with that name).
func (n *XNode) Find(name string) *XNode {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Clone deep-copies the node (ImportNode(..., True)).
func (n *XNode) Clone() *XNode {
	c := &XNode{Name: n.Name, HasText: n.HasText, Text: n.Text}
	for _, ch := range n.Children {
		c.Children = append(c.Children, ch.Clone())
	}
	return c
}

// TextOf returns the text of a child element ("" if missing).
func (n *XNode) TextOf(name string) string {
	if c := n.Find(name); c != nil {
		return c.Text
	}
	return ""
}

func xmlEscapeText(s string) string {
	if !strings.ContainsAny(s, "<>&\r") {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '\r':
			b.WriteString("&#13;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (n *XNode) write(b *bytes.Buffer, indent string) {
	b.WriteString(indent)
	switch {
	case len(n.Children) > 0:
		b.WriteString("<" + n.Name + ">" + LineEnding)
		for _, c := range n.Children {
			c.write(b, indent+"  ")
		}
		b.WriteString(indent + "</" + n.Name + ">" + LineEnding)
	case n.HasText:
		b.WriteString("<" + n.Name + ">" + xmlEscapeText(n.Text) + "</" + n.Name + ">" + LineEnding)
	default:
		b.WriteString("<" + n.Name + "/>" + LineEnding)
	}
}

// XMLDocBytes serializes a document with the given root (WriteXML).
func XMLDocBytes(root *XNode) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + LineEnding)
	if root != nil {
		root.write(&b, "")
	}
	return b.Bytes()
}

// ParseXML reads a document and returns the root element (ReadXMLFile).
// Whitespace-only text is dropped like FPC does with PreserveWhitespace=False.
func ParseXML(r io.Reader) (*XNode, error) {
	dec := xml.NewDecoder(r)
	var stack []*XNode
	var root *XNode
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := NewElem(t.Name.Local)
			if len(stack) > 0 {
				stack[len(stack)-1].Append(n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 && strings.TrimSpace(string(t)) != "" {
				top := stack[len(stack)-1]
				top.HasText = true
				top.Text += string(t)
			}
		}
	}
	if root == nil {
		return nil, errors.New("no root element")
	}
	return root, nil
}

// ReadXMLFile parses an XML file.
func ReadXMLFile(fileName string) (*XNode, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseXML(f)
}

// WriteXMLFile writes an XML document to a file.
func WriteXMLFile(root *XNode, fileName string) error {
	return os.WriteFile(fileName, XMLDocBytes(root), 0o644)
}
