# Changelog

## [0.14.0](https://github.com/morluto/gitcontribute/compare/v0.13.0...v0.14.0) (2026-07-29)


### Features

* **contribution:** add evidence-backed draft handoff ([c3ce58c](https://github.com/morluto/gitcontribute/commit/c3ce58ced769be8cb155a2fee54d8655dd252849))
* **contribution:** preserve proof-aware draft artifacts ([3cfed98](https://github.com/morluto/gitcontribute/commit/3cfed98e13dc57b133f3e27093b5b80405fabe7a))
* **tui:** add contribution research workflow ([5aa5d7e](https://github.com/morluto/gitcontribute/commit/5aa5d7e0b42f1b367814716e6d0096ccd9adbf50))
* **validation:** import external execution receipts ([6fc9341](https://github.com/morluto/gitcontribute/commit/6fc9341eff9ab567d1a4cb709517bcb1dd906ddd))
* **workflow:** derive issue contribution dispositions ([c240571](https://github.com/morluto/gitcontribute/commit/c240571839ae8ead4763ac316c59cbe8353b6608))


### Bug Fixes

* **config:** accept retired output settings ([1ed08ae](https://github.com/morluto/gitcontribute/commit/1ed08aec86b8ca25c015b9d3c78d76e53ed8f6fa))
* **config:** migrate retired output settings ([5099e85](https://github.com/morluto/gitcontribute/commit/5099e85fe63d2a852f3e439c2df58d3c6dad0404))
* **contribution:** satisfy draft validation lint ([9cf46a0](https://github.com/morluto/gitcontribute/commit/9cf46a0d8af97b63190a476f0d6aa21e8fd000f8))
* **github:** capture selected API version provenance ([8a2f9e5](https://github.com/morluto/gitcontribute/commit/8a2f9e5985ccadfa3ca8bd9d4be81e46432e1f1c))

## [0.13.0](https://github.com/morluto/gitcontribute/compare/v0.12.0...v0.13.0) (2026-07-28)


### Features

* **ci:** add release-please, gotestsum, integration tests, and harden repo ([21e0f34](https://github.com/morluto/gitcontribute/commit/21e0f34b2299b510a3dd132c901ee5ad5d6c0d24))
* **mcp:** batch repository research and expose recovery guidance ([434fd26](https://github.com/morluto/gitcontribute/commit/434fd26f48a695cbdc4e3c99c7544757cc232f0d))
* **mcp:** consolidate repository research reads ([badb5e8](https://github.com/morluto/gitcontribute/commit/badb5e89c60700643eba4a84c6b3bb073aeb7225))


### Bug Fixes

* **acquire:** shorten temporary mirror paths ([30b303d](https://github.com/morluto/gitcontribute/commit/30b303d8f60028167f93c441032c116b159c624d))
* **ci:** pass package list to gotestsum ([fd161be](https://github.com/morluto/gitcontribute/commit/fd161becc17a7ecb3bb13397f058b5166277934e))
* **ci:** split oversized sync files ([3357290](https://github.com/morluto/gitcontribute/commit/33572904125d4a410abbc57f1113921f62c70331))
* **github:** retry replayable reads safely ([c8e4266](https://github.com/morluto/gitcontribute/commit/c8e42665e381c9e8b28c538ba8dd1efaae60b699))
* **hydration:** refresh exact thread headers ([f152a42](https://github.com/morluto/gitcontribute/commit/f152a42c61a922a2a07229daff6e0ac53066e8d9))
* **jobs:** bound concurrent execution ([180128e](https://github.com/morluto/gitcontribute/commit/180128e12fb7797abc1a87c01b5e26183a15c869))
* **mcp:** remove obsolete facet aliases ([f883f40](https://github.com/morluto/gitcontribute/commit/f883f409a9a0f14f1815aa7e290543a79901fc52))
* **mcp:** return actionable validation errors ([5da4188](https://github.com/morluto/gitcontribute/commit/5da41880837614f3f4a1bdbd9e7c03ef31c01979))
* **npm:** mark launcher executable ([f3bed35](https://github.com/morluto/gitcontribute/commit/f3bed3559533c1827e35fbec2ca441f23fb65b52))
* **test:** close batch sync services ([7e1ba9d](https://github.com/morluto/gitcontribute/commit/7e1ba9d41b0dddc15496d5f0bf4dfd1ca9500a8b))


### Performance Improvements

* bound indexing, jobs, search, and clustering ([9c96943](https://github.com/morluto/gitcontribute/commit/9c96943dd07fb32efc2b8a7ea8791444e3f2ddc2))
* **clustering:** prune impossible duplicate pairs ([047d6a7](https://github.com/morluto/gitcontribute/commit/047d6a72888c1c6eae8ea20e3f68f5b77b88d620))
* **codeindex:** batch blob reads and reuse snapshots ([d78acd7](https://github.com/morluto/gitcontribute/commit/d78acd71a82a8f308097db6e16d8b2661a79d374))
* **jobs:** bound execution and batch cancellations ([c70dc60](https://github.com/morluto/gitcontribute/commit/c70dc60e1bcb9b03596cf59db7d76113519bc279))
* **mcp:** reduce payloads and batch corpus reads ([eaa944a](https://github.com/morluto/gitcontribute/commit/eaa944a13cf31a9ffd185fbcd9be47e30cd4d259))
* **search:** share one snapshot for pages and counts ([ce1fea8](https://github.com/morluto/gitcontribute/commit/ce1fea8db1a646236699ed7d77c5018c40250233))
