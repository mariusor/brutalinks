Feature: main page
    Visit main page and check elements.

    Scenario: Visit main page
        Given site is up
        When I visit "/"
        Then I should get the logo of "brutalinks (tech)"
