package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	_ "slowteetoe.com/presagio/Godeps/_workspace/src/gopkg.in/cq.v1"
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
	// log.Printf("Request %v", r)
	if err != nil {
		log.Printf("HTTP %d: %v", err)
		switch status {
		// We can have cases as granular as we like, if we wanted to
		// return custom errors for specific status codes.
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

	suggestions := FindSuggestions(q)

	response := Suggestion{Q: q, Suggestions: suggestions}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		panic(err)
	}
	return 200, nil
}

func cleanse(phrase string) string {
	return "^" + phrase
}

func FindSuggestions(phrase string) []string {
	q := cleanse(phrase)

	// have to keep track of dups (e.g. 4-gram model could suggest "to" as only suggestion, and 3-gram could suggest "to" also)
	keys := map[string]string{}

	var results []string

	for n := 4; n > 0; n-- {
		theseResults := findSuggestions(q, n)
		for _, t := range theseResults {
			if keys[t] == "" {
				results = append(results, t)
				keys[t] = t
			}
		}
		if len(results) >= 5 {
			return results[:5]
		}
	}
	if len(results) >= 5 {
		return results[:5]
	}
	return results
}

func findSuggestions(phrase string, ngramSize int) []string {
	log.Printf("Attempting to find suggestions from %v using a %v-gram\n", phrase, ngramSize)
	if ngramSize == 1 {
		log.Println("Returning default unigrams")
		return []string{"the", "to", "a", "i", "you"}
	}

	words := strings.Split(phrase, " ")

	q := phrase

	if len(words) > ngramSize {
		// only use the last n-1 words to predict since we only have 4-grams
		q = strings.Join(words[len(words)-(ngramSize-1):], " ")
	}

	log.Printf("Querying for: %v", q)

	rows, err := stmt.Query(q)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	results := make([]string, 0)

	var nextWord string
	var prob string
	for rows.Next() {
		err := rows.Scan(&nextWord, &prob)
		if err != nil {
			log.Fatal(err)
		}
		results = append(results, nextWord)
		log.Printf("%v -> %v (with probability %v)", q, nextWord, prob)
	}
	return results
}

var db *sql.DB
var stmt *sql.Stmt

func main() {
	db, err := sql.Open("neo4j-cypher", os.Getenv("GRAPHSTORY_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	stmt, err = db.Prepare(`
        match (a {phrase: {0} })-[p:PRECEDED]->(n) return n.word,p.p order by p.p desc limit 5
    `)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

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
