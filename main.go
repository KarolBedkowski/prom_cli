// Simple CLI Prometheus client
// Copyright (c) Karol Będkowski, 2016
// Licence: GPLv3+

package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	prometheus "github.com/prometheus/client_golang/api"
	apiv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"os"
	"sort"
	"time"
)

func formatCSV(rows [][]string, delim rune) string {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma = delim
	for _, row := range rows {
		w.Write(row)
		if err := w.Error(); err != nil {
			panic(err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		panic(err)
	}
	return b.String()
}

type dateFormatter func(ts model.Time) string

func simpleDateFormatter(ts model.Time) string {
	return ts.String()
}

type TimeByTime []model.Time

func (a TimeByTime) Len() int           { return len(a) }
func (a TimeByTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a TimeByTime) Less(i, j int) bool { return a[i] < a[j] }

func processMatrix(matrix model.Matrix, df dateFormatter) [][]string {
	rows := make(map[model.Time][]string)
	header := make([]string, 0, len(matrix))
	header = append(header, "timestamp")
	for i, stream := range matrix {
		header = append(header, stream.Metric.String())
		for _, value := range stream.Values {
			row := rows[value.Timestamp]
			for len(row) < i {
				row = append(row, "")
			}
			row = append(row, value.Value.String())
			rows[value.Timestamp] = row
		}
	}
	timestamps := make([]model.Time, 0, len(rows))
	for ts := range rows {
		timestamps = append(timestamps, ts)
	}

	sort.Sort(TimeByTime(timestamps))

	data := make([][]string, 0, len(rows)+1)
	data = append(data, header)
	for _, ts := range timestamps {
		row := []string{df(ts)}
		row = append(row, rows[ts]...)
		data = append(data, row)
	}
	return data
}

func processVector(vector model.Vector, df dateFormatter) [][]string {
	rows := make([][]string, 0, len(vector)+1)
	rows = append(rows, []string{"timestamp", "metric", "value"})
	for _, sample := range vector {
		rows = append(rows, []string{
			df(sample.Timestamp),
			sample.Metric.String(),
			sample.Value.String()})
	}
	return rows
}

func processScalar(scalar model.Scalar, df dateFormatter) [][]string {
	rows := [][]string{
		[]string{"timestamp", "value"},
		[]string{
			df(scalar.Timestamp),
			scalar.Value.String(),
		},
	}
	return rows
}

func main() {
	promURL := flag.String("url", "http://localhost:9090/", "prometheus url")
	promQuery := flag.String("query", "up", "prometheus query")
	promQueryRangeStart := flag.Int64("start", 0, "query range - start")
	promQueryRangeEnd := flag.Int64("end", 0, "query range - end")
	promQueryRangeStep := flag.Duration("step", 0, "query range - step")
	csvDelim := flag.String("delim", ";", "CSV field delimiter")
	csvDateFormat := flag.String("dateFormat", "2006-01-02 15:04:05",
		"Output date format; if empty or '-' = unix timestamp")
	flag.Parse()

	if *promQuery == "" {
		fmt.Println("error: missing query")
		return
	}
	if *promURL == "" {
		fmt.Println("error: missing prometheus url")
		return
	}

	clientConf := prometheus.Config{Address: *promURL}
	client, err := prometheus.NewClient(clientConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		return
	}

	api := apiv1.NewAPI(client)
	var value model.Value
	if *promQueryRangeStart > 0 {
		var end time.Time
		if *promQueryRangeEnd > 0 {
			end = time.Unix(*promQueryRangeEnd, 0)
		} else {
			end = time.Now()
		}
		step := *promQueryRangeStep
		if step <= 0 {
			step = time.Duration(5) * time.Minute
		}
		r := apiv1.Range{
			Start: time.Unix(*promQueryRangeStart, 0),
			End:   end,
			Step:  step,
		}
		value, err = api.QueryRange(context.Background(), *promQuery, r)
	} else {
		value, err = api.Query(context.Background(), *promQuery, time.Now())
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		return
	}

	var df dateFormatter
	if len(*csvDateFormat) > 0 && *csvDateFormat != "-" {
		df = func(ts model.Time) string {
			return time.Unix(0, int64(ts)*1000000).Format(*csvDateFormat)
		}
	} else {
		df = simpleDateFormatter
	}

	switch value.Type() {
	case model.ValMatrix:
		matrix := value.(model.Matrix)
		data := processMatrix(matrix, df)
		fmt.Println(formatCSV(data, rune((*csvDelim)[0])))
	case model.ValVector:
		vector := value.(model.Vector)
		data := processVector(vector, df)
		fmt.Println(formatCSV(data, rune((*csvDelim)[0])))
	case model.ValScalar:
		scalar := value.(*model.Scalar)
		data := processScalar(*scalar, df)
		fmt.Println(formatCSV(data, rune((*csvDelim)[0])))
	default:
		fmt.Fprintf(os.Stderr, "error: unknown/unimplemented type: %v\n", value.Type())
		fmt.Printf("Result:\n%+v\n", value)
	}
}
