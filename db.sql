CREATE table ratings (
    firstName varchar(255),
    lastName varchar(255),
    rating numeric(2, 1),
    date timestamp DEFAULT now(),
    PRIMARY KEY(firstName, lastName)
)