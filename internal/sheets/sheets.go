package sheets

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Client struct {
	svc *sheets.Service
}

func NewClient(ctx context.Context, credentialsFile string) (*Client, error) {
	b, err := readFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	config, err := google.JWTConfigFromJSON(b, sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	client := config.Client(ctx)
	svc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	return &Client{svc: svc}, nil
}

func readFile(path string) ([]byte, error) {
	return readFileFS(path)
}

type SheetData struct {
	Headers []string
	Rows    [][]interface{}
}

func (c *Client) ReadSheet(ctx context.Context, spreadsheetID, worksheet string) (*SheetData, error) {
	rangeStr := fmt.Sprintf("%s!A1:ZZZ", worksheet)
	resp, err := c.svc.Spreadsheets.Values.Get(spreadsheetID, rangeStr).Do()
	if err != nil {
		return nil, fmt.Errorf("read sheet %s/%s: %w", spreadsheetID, worksheet, err)
	}

	if len(resp.Values) == 0 {
		return &SheetData{}, nil
	}

	headers := make([]string, len(resp.Values[0]))
	for i, h := range resp.Values[0] {
		headers[i] = fmt.Sprintf("%v", h)
	}

	var rows [][]interface{}
	for _, row := range resp.Values[1:] {
		typedRow := make([]interface{}, len(headers))
		for j, cell := range row {
			if j < len(headers) {
				typedRow[j] = cell
			}
		}
		rows = append(rows, typedRow)
	}

	return &SheetData{Headers: headers, Rows: rows}, nil
}
