# About

This project represents a new attempt at the social link aggregator service. It is modelled after (old)Reddit, HackerNews, and Lobste.rs trying to combine the good parts of these services while mapping them on the foundation of an [ActivityPub](https://www.w3.org/TR/activitypub) generic service.

Targets small to medium communities which ideally focus on a single topic. At the same it allows reaching the "network effect" through the ability of federating with other similar services, but also with the rest of the fediverse ecosystem.

Built using a performant stack, and with minimal dependencies, we try to provide an easy out of the box installation. We provide standalone statically compiled binaries and docker containers. Even though having some developer experience is useful, we've tried to make deployment as easy as possible.

The community can be built using an invitation based model, where a user shares the responsibility for moderating the other accounts they invited to the service. The moderation actions are kept public and presented in an anonymized layout.

___

[![MIT Licensed](https://img.shields.io/github/license/mariusor/littr.go.svg)](https://raw.githubusercontent.com/mariusor/littr.go/master/LICENSE)
[![Builds status](https://builds.sr.ht/~mariusor/go-littr.svg)](https://builds.sr.ht/~mariusor/go-littr)
