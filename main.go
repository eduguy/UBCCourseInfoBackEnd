package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var globalDb *sql.DB

func updateDb(firstName string, lastName string, rating float64) {
	fmt.Println("update db with: ", firstName, lastName, rating)
	globalDb.QueryRowContext(context.Background(), "INSERT into ratings VALUES($1,$2,$3) ON CONFLICT (firstName, lastName) DO UPDATE SET date = EXCLUDED.date;", firstName, lastName, rating)
}

func getRating(c *gin.Context) {
	scraperChannel := make(chan float64)
	dbCacheChannel := make(chan float64)
	dbLastCheckedChannel := make(chan time.Time)

	firstName := c.Query("firstName")
	lastName := c.Query("lastName")

	go func() {
		resp, err := http.Get("http://54.193.122.205/rate?firstName=" + firstName + "&lastName=" + lastName)
		if err != nil {
			log.Fatalln(err)
		}
		//We Read the response body on the line below.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalln(err)
		}
		//Convert the body to type string
		sb := string(body)
		rating, err := strconv.ParseFloat(sb, 64)
		scraperChannel <- rating
	}()

	go func() {

		row := globalDb.QueryRowContext(context.Background(), "SELECT rating, date from ratings WHERE firstName = $1 AND lastName = $2", firstName, lastName)
		// if err != nil {
		// 	fmt.Println("db.QueryRowContext", err)
		// 	dbCacheChannel <- 0
		// 	dbLastCheckedChannel <- time.Unix(0, 0)
		// 	return
		// }
		var rating float64
		var date string

		if err := row.Scan(&rating, &date); err != nil {
			fmt.Println("row.Scan", err)
			dbCacheChannel <- 0
			dbLastCheckedChannel <- time.Unix(0, 0)
			return
		}

		dbCacheChannel <- rating
		dbLastChecked, err := time.Parse(time.RFC3339Nano, date)
		if err != nil {
			fmt.Println("parsing error")
			dbCacheChannel <- 0
			dbLastCheckedChannel <- time.Unix(0, 0)
			return
		}
		dbLastCheckedChannel <- dbLastChecked

	}()

	dbCacheResult := <-dbCacheChannel
	dbLastCheckedResult := <-dbLastCheckedChannel

	if !dbLastCheckedResult.AddDate(0, 0, 1).Before(time.Now()) {
		fmt.Println("db is new enough to return")
		c.IndentedJSON(http.StatusOK, dbCacheResult)
		go func() {
			scrapeResult := <-scraperChannel
			updateDb(firstName, lastName, scrapeResult)
		}()
		return
	}

	scrapeResult := <-scraperChannel
	fmt.Println("db is not new enough to return")
	// If by some miracle scrapeResult actually returns first, then there's no need to wait for the DB cache because we just want the most up to date
	c.IndentedJSON(http.StatusOK, scrapeResult)
	go func() {
		updateDb(firstName, lastName, scrapeResult)
	}()

}

func main() {

	dsn := url.URL{
		Host:   "localhost:5432",
		Path:   "postgres",
		User:   url.UserPassword("postgres", "12345"),
		Scheme: "postgres",
	}

	q := dsn.Query()
	q.Add("sslmode", "disable")

	dsn.RawQuery = q.Encode()

	db, err := sql.Open("pgx", dsn.String())

	if err != nil {
		fmt.Println("sql.Open", err)
		return
	}

	defer func() {
		db.Close()
		fmt.Println("closed")
	}()

	if err := db.PingContext(context.Background()); err != nil {
		fmt.Println("ping db failed")
		return
	}
	globalDb = db
	router := gin.Default()
	router.GET("/rating", getRating)
	router.Run("localhost:8080")
}
