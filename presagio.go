package main

import (
	"encoding/gob"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type Suggestion struct {
	Q           string   `json:"q"`
	Suggestions []string `json:"suggestions"`
}

type appHandler func(http.ResponseWriter, *http.Request) (int, error)

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// add CORS header
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	status, err := fn(w, r)

	if err != nil {
		log.Printf("HTTP %d: %v", err)
		switch status {
		case http.StatusNotFound:
			http.NotFound(w, r)
		case http.StatusInternalServerError:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		default:
			// Catch any other errors we haven't explicitly handled
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}

func suggestionsHandler(w http.ResponseWriter, r *http.Request) (int, error) {

	q := r.FormValue("q")

	s := FindSuggestions(q)

	response := Suggestion{Q: q, Suggestions: s}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		panic(err)
	}
	return 200, nil
}

func cleanse(phrase string) string {
	clean := alphaspace.ReplaceAll([]byte(phrase), []byte(""))
	clean = spaces.ReplaceAll(clean, []byte(" "))
	return "^" + strings.ToLower(string(clean))
}

func FindSuggestions(phrase string) []string {
	nResults := 3
	q := cleanse(phrase)

	// have to keep track of dups (e.g. 4-gram model could suggest "to" as only suggestion, and 3-gram could suggest "to" also)
	keys := map[string]string{}

	var results []string

	for n := 4; n > 0; n-- {
		theseResults := findSuggestions(q, n)
		for _, t := range theseResults {
			if keys[t] == "" && t != "'" {
				results = append(results, t)
				keys[t] = t
			}
		}
		if len(results) >= nResults {
			return results[:nResults]
		}
	}
	if len(results) >= nResults {
		return results[:nResults]
	}
	return results
}

func findSuggestions(phrase string, ngramSize int) []string {
	if ngramSize == 1 {
		log.Println("Returning default unigrams")
		return []string{"the", "to", "a"}
	}

	words := strings.Split(phrase, " ")

	q := phrase

	if len(words) > ngramSize {
		// only use the last n-1 words to predict since we only have 4-grams
		q = strings.Join(words[len(words)-(ngramSize-1):], " ")
	}

	log.Printf("Attempting to find suggestions for [%v] using a %v-gram, searching map for [%v]\n", phrase, ngramSize, q)

	v, ok := m[q]
	if !ok {
		log.Println("Nothing found.")
		return []string{}
	}

	log.Printf("Returning %v", v.Words)
	return v.Words
}

type Suggestions struct {
	Words []string
}

var m map[string]Suggestions

var alphaspace = regexp.MustCompile("[^[:alpha:][:space:]']")
var spaces = regexp.MustCompile("[[:space:]]+")

func main() {

	// open the stored hashmap
	decodeFile, err := os.Open("ngrams.gob")
	if err != nil {
		panic(err)
	}
	defer decodeFile.Close()

	decoder := gob.NewDecoder(decodeFile)

	m = make(map[string]Suggestions)

	decoder.Decode(&m)

	log.Println("Reloaded ngram map from file")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("$PORT was unset, defaulting to %v", port)
	}
	s := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}
	http.Handle("/", appHandler(suggestionsHandler))
	http.HandleFunc("/favicon.ico", http.NotFound)
	log.Fatal(s.ListenAndServe())
}
