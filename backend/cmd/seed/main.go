// seed populates the player database from the ranking.html HTML fixture.
// Usage: go run ./cmd/seed --fixture ../../scraper/fixtures/ranking.html --db ../../data/players.db
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/tobiasmarkusbraun/rgblitz/internal/db"
	"github.com/tobiasmarkusbraun/rgblitz/internal/scraper"
)

func main() {
	fixturePath := flag.String("fixture", "../../scraper/fixtures/ranking.html", "path to ranking.html fixture")
	dbPath := flag.String("db", "../../data/players.db", "path to sqlite database")
	flag.Parse()

	f, err := os.Open(*fixturePath)
	if err != nil {
		log.Fatalf("open fixture %q: %v", *fixturePath, err)
	}
	defer f.Close()

	open, err := scraper.ParseRankingPage(f)
	if err != nil {
		log.Fatalf("parse ranking page: %v", err)
	}
	players := scraper.MergeWomenRGX(open, nil)
	fmt.Printf("parsed %d players from fixture\n", len(players))

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlayerStore(database)
	added, updated, errs := 0, 0, 0
	for _, p := range players {
		existing, _ := store.GetByID(p.ID)
		if err := store.Upsert(p); err != nil {
			log.Printf("upsert %s: %v", p.ID, err)
			errs++
			continue
		}
		if existing == nil {
			added++
		} else {
			updated++
		}
	}
	fmt.Printf("seed complete: %d added, %d updated, %d errors\n", added, updated, errs)
}
