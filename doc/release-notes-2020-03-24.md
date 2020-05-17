## The abnormally long vacation release

Today I deployed the new version of littr.me that I've been working on for some months. It mainly consists of lowering the reliance on the non standard ActivityPub collections that fedbox exposes, namely `/actors`, `/activities` and `/objects`, especially the later.

An overview of the specific changes in the way we load items can be seen in the [docs/c2s.md](https://github.com/mariusor/littr.go/blob/master/doc/c2s.md#querying-fedbox) document in the littr.me repository. 

Unfortunatelly this has brought some loss of information as not all submissions conformed to the way we're sorting the `Create` activities currently. They can still be found in the activities collection, but at creation time they haven't been addressed to the instance itself, so they're missing from its inbox, which is what we're now using for loading items on the main page.

This new version contains better `Follow` requests handling, on both the accounts that receive and send them.

