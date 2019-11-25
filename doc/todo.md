# Issues
* When adding a new OAuth2 client from the command line, 
we shouldn't allow password flow by default, but based on a parameter when creating it.
* Refactor the fedbox API client and the filters overall. Main issue currently:
    When loading an Activity collection, dereference the Objects in it and load those from the /objects end-point 
* Audience improvements:
    * Add all @mentions to the CC field
    * ~~Move local instance from To to BCC field~~
    * ~~Add the attributedTo of the item replied to, to the To field~~
* ~~Fix @mentions and #tags parsing.~~
* ~~Going to a reply, doesn't load it's children.~~
* ~~Registered actors are missing quite a lot of fields: `Published`, `Updated`, `Endpoints`, `Url`.~~
* ~~All children objects should be added to the OP's replies collection.~~
* ~~After submitting a Like/Dislike, it seems we can't Undo or do the reverse of it.~~
* ~~A logged in user seems to be able to be allowed to edit/delete Anonymous objects.~~
* ~~A logged in user doesn't seem to be able to edit/delete his own objects.~~
