# Querying FedBOX

## Loading main page items

Loads FedBOX's service inbox `/inbox`

## Loading federated tab items

Same as main page items, but the filtering should have the base IRI different than fedbox's host.[1]

## Loading followed items

Loads the logged account's inbox. `/actors/{uuid}/inbox`

## Load discussions

Loads the `/objects` end-point with a filter for Url being empty and Context being empty.

## Load items from a particular domain

Loads the `/objects` end-point with a filter on Url to match the required domain.

## Load a user's items

Loads the outbox of the actor corresponding to the user: `/actors/{uuid}/outbox`

## Load a user's votes

Loads the liked end-point of the actor corresponding to the user: `/actors/{uuid}/liked`

## Loading an item's comments

Loads the item's Context property (which can be it's top level item, or itself) and loads the `/objects/{uuid}/replies`

## Loading an item's votes

Loads the item's like collection `/objects/{uuid}/likes`

