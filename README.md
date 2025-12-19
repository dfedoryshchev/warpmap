# warpmap

map a codebase before you change it.

warpmap ranks the files that carry the most risk - the ones that change often AND are complex -
so you (or an agent) know where to look before touching an unfamiliar system. the principle is
boring and reliable: understand what you are standing on before you move it.

## status

early. `hotspots` works today; a multi-language import graph and blast-radius trace are next.

## usage

```
warpmap hotspots ./my-project
```
