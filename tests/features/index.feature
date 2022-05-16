Feature: main page
    Visit main page

    Scenario: Visit main page
        Given site is up
        When I visit /
        Then I should get status "200 OK"
