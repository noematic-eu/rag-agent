package main

// DocumentTFIDF représente un document avec ses scores TF-IDF (legacy finalize path).
type DocumentTFIDF struct {
	ID    string
	TFIDF map[string]float64
	Norm  float64
}

var documentTFIDFs []DocumentTFIDF
var globalIDF map[string]float64

func init() {
	globalIDF = make(map[string]float64)
}
