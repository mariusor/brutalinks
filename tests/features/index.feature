Feature: main page
    Visit main page

    Scenario: Visit main page
        Given site is up
        When I visit "/"
        Then I should get the logo of "brutalinks (tech)"
