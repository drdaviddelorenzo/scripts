package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// You need these files present to run the check.
const (
	// Save https://www.23andme.com/you/download/
	// Unzip it, and rename genome_Your_Name_Full_timestamp.txt -> rawdata.txt
	RAWDATA_FILENAME = "rawdata.txt"
	// curl https://api.23andme.com/1/genomes/192889f1/ > apidata.txt
	API_DATA_FILENAME = "apidata.txt"
	// curl https://api.23andme.com/res/txt/snps.data
	KEY_FILENAME = "snps.data"
)

var (
	FALSE_ALARMS = map[string]bool{
		"AA|A": true,
		"CC|C": true,
		"GG|G": true,
		"TT|T": true,
		"DD|D": true,
		"II|I": true,
		"__|":  true,
		"--|":  true,
	}
)

type CallPair struct {
	ApiCall     string
	RawDataCall string
}

type Mismatch struct {
	CallPair
	Count int
}

type GenomesEndpoint struct {
	Id     string
	Genome string
}

type SNP string

type Mismatches []Mismatch

// For sorting
func (m Mismatches) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m Mismatches) Len() int           { return len(m) }
func (m Mismatches) Less(i, j int) bool { return m[i].Count > m[j].Count }

func getSNPstoCall() *map[string]string {
	var (
		file *os.File
		line []byte
		err  error
	)
	if file, err = os.Open(RAWDATA_FILENAME); err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	SNPtoCall := make(map[string]string, 1050000)
	for {
		if line, _, err = reader.ReadLine(); err != nil {
			break
		}
		linestring := string(line)
		if strings.HasPrefix(linestring, "#") {
			continue
		}
		val := strings.Split(linestring, "\t")
		SNPtoCall[val[0]] = val[3]
	}
	return &SNPtoCall
}

func getIndexToSNP() *map[int64]string {
	var (
		file *os.File
		line []byte
		err  error
	)
	indexToSNP := make(map[int64]string, 1050000)
	if file, err = os.Open(KEY_FILENAME); err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	for {
		if line, _, err = reader.ReadLine(); err != nil {
			break
		}
		linestring := string(line)
		if strings.HasPrefix(linestring, "#") || strings.HasPrefix(linestring, "index") {
			continue
		}
		val := strings.Split(linestring, "\t")
		var index int64
		if index, err = strconv.ParseInt(val[0], 10, 32); err != nil {
			break
		}
		indexToSNP[index] = val[1]
	}
	return &indexToSNP
}

func getCallpairs(indexToSNP *map[int64]string,
	SNPtoCall *map[string]string) (callpairs map[CallPair][]SNP, correct, incorrect int) {
	var err error
	callpairs = make(map[CallPair][]SNP, 10)
	jsondata, err := ioutil.ReadFile(API_DATA_FILENAME)
	if err != nil {
		log.Fatal(err)
	}
	var genomes GenomesEndpoint
	json.Unmarshal(jsondata, &genomes)
	for index := 0; index < len(genomes.Genome); index += 2 {
		api_call := fmt.Sprintf("%s%s", string(genomes.Genome[index]), string(genomes.Genome[index+1]))
		snpstr, _ := (*indexToSNP)[int64(index/2)]
		raw_data_call, _ := (*SNPtoCall)[snpstr]
		snp := SNP(snpstr)
		// Add mismatches; some are not true mismatches
		false_alarm := FALSE_ALARMS[fmt.Sprintf("%s|%s", api_call, raw_data_call)]
		if (api_call != raw_data_call) && !false_alarm {
			callpair := CallPair{ApiCall: api_call, RawDataCall: raw_data_call}
			if _, found := callpairs[callpair]; !found {
				callpairs[callpair] = []SNP{snp}
			} else {
				callpairs[callpair] = append(callpairs[callpair], snp)
			}
			incorrect += 1
		} else {
			correct += 1
		}
	}
	return
}

func printAndCalculateMismatches(callpairs map[CallPair][]SNP, correct, incorrect int) {
	mismatches := Mismatches{}
	for callpair, snps := range callpairs {
		mismatch := Mismatch{CallPair: CallPair{ApiCall: callpair.ApiCall, RawDataCall: callpair.RawDataCall}, Count: len(snps)}
		mismatches = append(mismatches, mismatch)
	}
	sort.Sort(mismatches)
	for _, mismatch := range mismatches {
		fmt.Printf("ApiCall: %s\tRawDataCall: %s\tTotal: %d\t\n", mismatch.ApiCall, mismatch.RawDataCall, mismatch.Count)
		buffer := bytes.Buffer{}
		buffer.WriteString("SNPS: ")
		for i, snp := range callpairs[mismatch.CallPair] {
			buffer.WriteString(fmt.Sprintf("%s, ", snp))
			if (i%6 == 0) && (i > 0) {
				buffer.WriteString("\n")
			}
		}
		buffer.WriteString("\n\n")
		fmt.Print(buffer.String())
	}
	fmt.Printf("Same: %d, Mismatches: %d, Same: %f%%", correct, incorrect, float32(correct)/float32(incorrect+correct)*100)
}

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	done := make(chan bool)
	var (
		SNPtoCall  *map[string]string
		indexToSNP *map[int64]string
	)
	go func() {
		SNPtoCall = getSNPstoCall()
		done <- true
	}()
	go func() {
		indexToSNP = getIndexToSNP()
		done <- true
	}()
	for i := 0; i < 2; i++ {
		<-done
	}
	callpairs, correct, incorrect := getCallpairs(indexToSNP, SNPtoCall)
	printAndCalculateMismatches(callpairs, correct, incorrect)
}
