# Go Backend for UBC Course Info

Within the UBCCourseInfo repo, there is already a simple back-end server using Python and Flask to scrape results on user request. However, this approach didn't leverage a database to store ratings, resulting in a slow user experience.

This code implements a more intelligent Go server that will concurrently:
- Send a request to the scraper server for new results
- Query a PostGres database that maintains Professor ratings

Depending on how recently the postgres entry was created, it will either block and wait for new results from the scraper or immediately return the value from the database. Either way, after the response has been sent, it will update the database to speed up future requests.