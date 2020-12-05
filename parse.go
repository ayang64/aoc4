package main

import (
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"unicode"
)

type Token struct {
	Type  rune
	Value interface{}
}

const (
	TokenKVP = rune(iota + 0xe000)
	TokenEndOfBlock
	TokenCollection
)

func (t Token) String() string {
	smap := map[rune]string{
		TokenKVP:        "TokenKVP",
		TokenEndOfBlock: "TokenEndOfBlock",
		TokenCollection: "TokenCollection",
	}

	s, ok := smap[t.Type]

	if !ok {
		return "*unknown-token*"
	}

	return s
}

type Scanner struct{}

func (s *Scanner) match(rs io.RuneScanner, m func(rune) (bool, bool, error)) (string, error) {
	sb := strings.Builder{}
	for {
		r, _, err := rs.ReadRune()
		if err != nil {
			return sb.String(), err
		}

		accept, cont, err := m(r)

		if err != nil {
			rs.UnreadRune()
			break
		}

		if accept {
			sb.WriteRune(r)
		}

		if !cont {
			break
		}
	}
	return sb.String(), nil
}

func (s *Scanner) scan(rs io.RuneScanner, tc chan<- Token) {
	log.Printf("scanning...")

	for {
		// peek at next rune
		r, _, err := rs.ReadRune()
		if err != nil {
			tc <- Token{Type: TokenEndOfBlock, Value: string(r)}
			return
		}
		rs.UnreadRune()

		switch {
		case r == '\n':
			log.Printf("newline!")
			n := 0
			s, _ := s.match(rs, func(r rune) (bool, bool, error) {
				log.Printf("matching %v", r)
				if r != '\n' {
					return false, false, fmt.Errorf("not a newline")
				}
				n++
				return true, true, nil
			})

			log.Printf("MATCHED %d %q ", n, s)

			if s == "\n\n" {
				tc <- Token{Type: TokenEndOfBlock, Value: string(r)}
			}
		case unicode.IsSpace(r):
			rs.ReadRune()
		default:
			s, _ := s.match(rs, func(r rune) (bool, bool, error) {
				if unicode.IsSpace(r) {
					return false, false, fmt.Errorf("not an atom rune")
				}
				return true, true, nil
			})

			if s != "" {
				tc <- Token{Type: TokenKVP, Value: s}
			}
		}
	}
}

func (s *Scanner) Scan(rs io.RuneScanner) <-chan Token {
	rc := make(chan Token)
	log.Printf("Scan()")
	go func() { s.scan(rs, rc); close(rc) }()
	return rc
}

func Parse(rs io.RuneScanner) int {
	scn := Scanner{}
	toks := []Token{}
	valid := 0

	for tok := range scn.Scan(rs) {
		log.Printf("--------> %s", tok)
		toks = append(toks, tok)

		end := len(toks) - 1

		log.Printf("TOKS: %+v", toks)

		switch {
		case len(toks) > 1 && toks[end-1].Type == TokenCollection && toks[end].Type == TokenEndOfBlock:
			collection := toks[end-1].Value.(map[string]string)
			log.Printf("HANDLING COLLECTION %v", collection)
			toks = toks[:len(toks)-2]

			validator := map[string]func(string) bool{
				"byr": func(s string) bool {
					v, err := strconv.ParseInt(s, 10, 64)
					if err != nil {
						return false
					}
					return v >= 1920 && v <= 2002
				},
				"iyr": func(s string) bool {
					v, err := strconv.ParseInt(s, 10, 64)
					if err != nil {
						return false
					}
					return v >= 2010 && v <= 2020
				},
				"eyr": func(s string) bool {
					v, err := strconv.ParseInt(s, 10, 64)
					if err != nil {
						return false
					}
					return v >= 2020 && v <= 2030
				},
				"hgt": func(s string) bool {
					height := 0
					unit := ""
					fmt.Sscanf(s, "%d%s", &height, &unit)
					log.Printf("height = %v, unit = %v", height, unit)
					rc := (unit == "cm" && height >= 150 && height <= 193) || (unit == "in" && height >= 59 && height <= 76)

					log.Printf("hgt:%s returning %v", s, rc)
					return rc
				},
				"hcl": func(s string) bool {
					hcl := 0
					m, err := fmt.Sscanf(s, "#%x", &hcl)
					log.Printf("HCL MATCH %v", m)
					if err != nil {
						return false
					}
					return len(s) == 7
				},
				"ecl": func(s string) bool {
					valid := map[string]struct{}{
						"amb": {},
						"blu": {},
						"brn": {},
						"gry": {},
						"grn": {},
						"hzl": {},
						"oth": {},
					}
					_, exists := valid[s]
					return exists
				},
				"pid": func(s string) bool {
					v := 0
					m, err := fmt.Sscanf(s, "%d", &v)
					log.Printf("PID: s = %s, len(s) = %d, m = %v, err = %v", s, len(s), m, err)
					rc := err == nil && m == 1 && len(s) == 9
					log.Printf("pid returning %v", rc)
					return rc
				},
				"cid": func(s string) bool { return true },
			}

			matches := 0
			for _, key := range []string{"byr", "iyr", "eyr", "hgt", "hcl", "ecl", "pid", "cid"} {
				if _, exists := collection[key]; !exists {
					continue
				}

				f, exists := validator[key]
				if !exists {
					continue
				}

				log.Printf("%s:%s valid %v", key, collection[key], f(collection[key]))
				if f(collection[key]) {
					matches++
				}
			}

			if _, hascid := collection["cid"]; matches == 8 || matches == 7 && !hascid {
				valid++
			}

		case len(toks) > 1 && toks[end-1].Type == TokenCollection && toks[end].Type == TokenKVP:
			collection := toks[end-1].Value.(map[string]string)
			value := toks[end].Value.(string)
			idx := strings.Index(value, ":")
			collection[value[:idx]] = value[idx+1:]

			log.Printf("ADDING KVP TO COLLECTION: collection: %+v, kvp = %q", collection, value)

			toks = toks[:len(toks)-1]
			log.Printf("SHIFTING NEW KVP")

		case len(toks) == 1 && toks[end].Type == TokenKVP:
			value := toks[end].Value.(string)
			idx := strings.Index(value, ":")
			collection := map[string]string{
				value[:idx]: value[idx+1:],
			}
			log.Printf("CREATING NEW KVP")
			log.Printf("BEFORE SHIFT: %+v", toks)
			toks[end] = Token{Type: TokenCollection, Value: collection}
			log.Printf("AFTER SHIFT: %+v", toks)
		}
	}

	return valid
}
