package payload

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Generator struct {
	template string
	csvRows  []map[string]string
	rowIdx   atomic.Int64
	mu       sync.RWMutex
	rng      *rand.Rand
}

func NewGenerator(template string, csvData string) (*Generator, error) {
	g := &Generator{
		template: template,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if csvData != "" {
		rows, err := parseCSV(csvData)
		if err != nil {
			return nil, fmt.Errorf("parse csv: %w", err)
		}
		g.csvRows = rows
	}

	return g, nil
}

func (g *Generator) Generate() ([]byte, error) {
	result := g.template

	result = g.replaceFakerTokens(result)
	result = g.replaceCSVTokens(result)

	var js json.RawMessage
	if err := json.Unmarshal([]byte(result), &js); err != nil {
		return []byte(result), nil
	}
	return js, nil
}

func (g *Generator) replaceFakerTokens(s string) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	replacements := map[string]func() string{
		"{{faker.uuid}}":      func() string { return fakeUUID(g.rng) },
		"{{faker.email}}":     func() string { return fakeEmail(g.rng) },
		"{{faker.name}}":      func() string { return fakeName(g.rng) },
		"{{faker.firstName}}": func() string { return fakeFirstName(g.rng) },
		"{{faker.lastName}}":  func() string { return fakeLastName(g.rng) },
		"{{faker.phone}}":     func() string { return fakePhone(g.rng) },
		"{{faker.int}}":       func() string { return fmt.Sprintf("%d", g.rng.Intn(100000)) },
		"{{faker.username}}":  func() string { return fakeUsername(g.rng) },
		"{{faker.sentence}}":  func() string { return fakeSentence(g.rng) },
		"{{faker.timestamp}}": func() string { return time.Now().UTC().Format(time.RFC3339) },
		"{{faker.bool}}":      func() string { return fmt.Sprintf("%t", g.rng.Intn(2) == 1) },
	}

	for token, fn := range replacements {
		for strings.Contains(s, token) {
			s = strings.Replace(s, token, fn(), 1)
		}
	}

	return s
}

func (g *Generator) replaceCSVTokens(s string) string {
	if len(g.csvRows) == 0 {
		return s
	}
	idx := int(g.rowIdx.Add(1)-1) % len(g.csvRows)
	row := g.csvRows[idx]
	for key, val := range row {
		s = strings.ReplaceAll(s, fmt.Sprintf("{{csv.%s}}", key), val)
	}
	return s
}

func parseCSV(data string) ([]map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv must have header and at least one row")
	}
	headers := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string)
		for i, h := range headers {
			if i < len(record) {
				row[h] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

var firstNames = []string{"Alex", "Maria", "Ivan", "Elena", "Dmitry", "Anna", "Sergei", "Olga", "Nikita", "Yuri", "Max", "Julia", "Roman", "Kate", "Andrei"}
var lastNames = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez", "Lee", "Kim", "Wang", "Chen", "Petrov"}
var domains = []string{"gmail.com", "yahoo.com", "test.io", "example.com", "mail.dev"}
var words = []string{"quick", "brown", "fox", "jumps", "over", "lazy", "dog", "the", "hello", "world", "load", "test", "data", "payload", "service"}

func fakeUUID(r *rand.Rand) string {
	b := make([]byte, 16)
	r.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func fakeEmail(r *rand.Rand) string {
	return fmt.Sprintf("%s.%s%d@%s",
		strings.ToLower(firstNames[r.Intn(len(firstNames))]),
		strings.ToLower(lastNames[r.Intn(len(lastNames))]),
		r.Intn(999),
		domains[r.Intn(len(domains))])
}

func fakeName(r *rand.Rand) string {
	return fmt.Sprintf("%s %s", fakeFirstName(r), fakeLastName(r))
}

func fakeFirstName(r *rand.Rand) string { return firstNames[r.Intn(len(firstNames))] }
func fakeLastName(r *rand.Rand) string  { return lastNames[r.Intn(len(lastNames))] }

func fakePhone(r *rand.Rand) string {
	return fmt.Sprintf("+7%010d", r.Int63n(10000000000))
}

func fakeUsername(r *rand.Rand) string {
	return fmt.Sprintf("%s_%s%d",
		strings.ToLower(firstNames[r.Intn(len(firstNames))]),
		strings.ToLower(lastNames[r.Intn(len(lastNames))]),
		r.Intn(99))
}

func fakeSentence(r *rand.Rand) string {
	n := 4 + r.Intn(6)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = words[r.Intn(len(words))]
	}
	if len(parts[0]) > 0 {
		parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	}
	return strings.Join(parts, " ") + "."
}
