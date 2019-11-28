# Querying FedBOX

Unless specified all filters mention in [FedBOX](https://github.com/go-ap/fedbox/tree/master/doc/c2s.md) apply to the 
corresponding types of collections we're loading from.

In the following scenarios, to get the full set of information we usually require more than the one request mentioned in them.

For fully populating a littr.me Item, we require the following information besides the ActivityPub Object itself:

* The item's author data.

This means that for every items collection we are obtaining from fedbox, we must dereference and load all the `submittedBy` or `Actor` IRIs.
This is done by loading the `/actors` end point with an IRI filter for all of the actor IRIs we want to dereference.

eg: https://federated.id/actors?iri=https://federated.id/actors/{uuid1}&iri=https://federated.id/actors/{uuid2}

* The item's votes data.

This means that for every item collection, we must load all Like/Dislike/Undo Activities that have said item as an Object.
This is done by loading the `/activities` end point with an Object filter for all the IRIs of the items.

eg: https://federated.id/activities?type=Like&type=Dislike&type=Undo&object=https://federated.id/objects/{uuid1}&object=https://federated.id/object/{uuid2}

For getting the full information about a (logged in) user, we also need to load his votes. 

The request to get them, should load the actor's `liked` collection where the object filter matches the IRIs of the objects we want to know the votes on.

## Loading main page items

Loads FedBOX's service inbox `/inbox`

## Loading federated tab items

Same as main page items, but the filtering should have the base IRI different than fedbox's host.

**!** This isn't implemented yet, as fedbox doesn't support negating filter values.

## Loading followed items

Loads the logged account's inbox. `/actors/{uuid}/inbox`

## Load discussions

Loads the `/objects` end-point with a filter for Url being empty and Context being empty.

It doesn't work by accessing the `/inbox` because it is an Activity collection, and we can't filter by the object's properties.

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

# Saving to FedBOX


