I need a feed reader that can import my list from Reeder app (you can see the format here: ./Feeds.opml)

My criterias
Web based
Singple port for both BE and FE
singple docker-compose bring up the whole thing.
No auth needed for now or login
can update and konw what I have already read
having a nice UI similar to other Feed Readers.
Aboity to proect the feed via AI. to do so instead of adding an AI there I prefer ability to export things like, Today Reads, This Week Reads and get the feed in Markdown foramt and sedn to another AI, chatbot
or other to process. Feed must come with link back to original articals and also their link in our feed reader just in case if I want to link/open the sources. just in case if you had a better idea let me know.
Having a nice UI and great usr experience is must.
note: for development I do not want to install stuff locally. as I said all must be dockerized nicely and solution run via docker-compose
for BE I prefer Go. for persistence you pick sqlite or PG.

feed management from the UI. add a feed by url, remove or rename a feed, move it to a folder. when a feed errors (404 etc) let me see the error and retry just that feed.

mark as read on scroll, like Reeder on ios. when an unread article scrolls above the top of the list mark it read even if I never opened it. make it optional, some ppl hate it.

a settings/config section so I can toggle stuff like the scroll behaviour, list density, default filter.

keyboard shortcuts. and pressing ? should popup the list of shortcuts becasue my memory is bad.

mark all as read for whatever I have selected (all articles, a folder, or a single feed).

export must respect what I have selected (folder/feed) and use MY timezone for today/this week not UTC.
