# Changelog

## [3.0.1](https://github.com/morluto/gitcontribute/compare/v3.0.0...v3.0.1) (2026-08-10)


### Bug Fixes

* **release:** stop gating npm discovery on redundant npx round-trip ([b710e0f](https://github.com/morluto/gitcontribute/commit/b710e0f42a0b14d71dd410bebd9fbf5bf763b24a))
* **release:** stop gating npm discovery on redundant npx round-trip ([95c3c84](https://github.com/morluto/gitcontribute/commit/95c3c8467da5c957f8f8088531e1a934bdf92cb0))

## [3.0.0](https://github.com/morluto/gitcontribute/compare/v2.0.0...v3.0.0) (2026-08-09)


### ⚠ BREAKING CHANGES

* **mcp:** corpus.search_code_batch is now corpus.search_code, corpus.list_pull_requests is now corpus.search_pull_requests, github.sync_ci_failures is now github.sync_pull_request_ci, and prescriptive composite workflow tools are no longer advertised.

### Features

* add explainable offline Contribution Radar ([#7](https://github.com/morluto/gitcontribute/issues/7)) ([ebc561c](https://github.com/morluto/gitcontribute/commit/ebc561c524972d9475b69b2990ff4423af4051ad))
* add thread research briefs ([#8](https://github.com/morluto/gitcontribute/issues/8)) ([82bafec](https://github.com/morluto/gitcontribute/commit/82bafec79a9b24c9f237ba0b587fbc840c47cc92))
* **analysis:** add versioned similarity and bounded ranking ([4769339](https://github.com/morluto/gitcontribute/commit/4769339926d527e4e6fb5a60baf07182ea09aeb0))
* **app,github,cli:** add structured logging, circuit breaker, and trace IDs ([bb3cbfc](https://github.com/morluto/gitcontribute/commit/bb3cbfc8a8c682c7c0ac1892f825166bb8a719c4))
* **buflimit:** extract bounded output buffer and adopt in git runners ([ee3d848](https://github.com/morluto/gitcontribute/commit/ee3d848d861d6721c48beea91496ed5870333e35))
* **ci,dx:** add golangci-lint, pre-commit hooks, CI hardening, and agent readiness improvements ([0e70dd9](https://github.com/morluto/gitcontribute/commit/0e70dd953f5243d0c567de546cd7b671956d3d1e))
* **ci:** add release-please, gotestsum, integration tests, and harden repo ([21e0f34](https://github.com/morluto/gitcontribute/commit/21e0f34b2299b510a3dd132c901ee5ad5d6c0d24))
* **commitplan:** add hunk-aware semantic commit planning ([8635b00](https://github.com/morluto/gitcontribute/commit/8635b003a409538694a3785f9a0b60167192c081))
* **commitplan:** expose read-only semantic planning ([56f01ef](https://github.com/morluto/gitcontribute/commit/56f01ef6e7d77200ca5d9affd0a1793068f976b9))
* **commitplan:** inventory and verify workspace changes ([d45ed47](https://github.com/morluto/gitcontribute/commit/d45ed471750f33629970ba9b4f4586d1df983cc5))
* **concern:** add a repo-local seed concern ledger ([12400cc](https://github.com/morluto/gitcontribute/commit/12400cc3ecfd7c44705138a6a6f84087c6407cdb))
* **concern:** add local concern ledger ([689f689](https://github.com/morluto/gitcontribute/commit/689f689eb1ddc8eb1c3fc9ba6403896470de5b4a))
* **concern:** expose local concern workflows ([bdfb5e4](https://github.com/morluto/gitcontribute/commit/bdfb5e4139480a6e4a6110d18d869aef69dc5bb6))
* **contribution:** add evidence-backed draft handoff ([c3ce58c](https://github.com/morluto/gitcontribute/commit/c3ce58ced769be8cb155a2fee54d8655dd252849))
* **contribution:** preserve proof-aware draft artifacts ([3cfed98](https://github.com/morluto/gitcontribute/commit/3cfed98e13dc57b133f3e27093b5b80405fabe7a))
* **core:** add scalable contribution research primitives ([073d66e](https://github.com/morluto/gitcontribute/commit/073d66e5ab6e590950a4369ad6509a497b8d335b))
* **corpus,app:** avoid materializing threads for repository counts ([3050001](https://github.com/morluto/gitcontribute/commit/305000133a284238d7b803c512697591fe070eb6))
* **corpus,cli,app:** bounded list results and similarity windows ([2b5933a](https://github.com/morluto/gitcontribute/commit/2b5933ab818d3d5f79fd6285b7fce9288b3cbdfe))
* **corpus:** add first-class actor projections ([b8441f3](https://github.com/morluto/gitcontribute/commit/b8441f3ee4d04b9dfd455137489661123a5a6a08))
* **corpus:** add pull-request feedback projections ([d1a5c10](https://github.com/morluto/gitcontribute/commit/d1a5c109f26e2c812ceab9ac426d31359219cadf))
* **corpus:** bind offline reads to corpus revisions ([6e75235](https://github.com/morluto/gitcontribute/commit/6e7523533a213ae7dbc4827bb36caaf97f3c9136))
* **corpus:** enforce schema lineage hard cutover ([471ff51](https://github.com/morluto/gitcontribute/commit/471ff51bde0d1ab86ae309c0de205c5d31665668))
* **corpus:** enforce schema lineage hard cutover ([e8df231](https://github.com/morluto/gitcontribute/commit/e8df231bd87079de9310b72cae41b52ffb4d4dac))
* **corpus:** persist immutable snapshot and index identities ([d1f41e0](https://github.com/morluto/gitcontribute/commit/d1f41e0aedff074bdb6ea290ccebfc1283ae666b))
* **corpus:** separate lifecycle ownership and recovery ([2bdeec9](https://github.com/morluto/gitcontribute/commit/2bdeec99f944ab74debd4362e3cf66b3016451e0))
* **corpus:** separate lifecycle ownership and recovery ([35fec66](https://github.com/morluto/gitcontribute/commit/35fec661933cef91c84d1c56c961dffea323199b))
* **github:** add bounded actor acquisition adapters ([e3ab595](https://github.com/morluto/gitcontribute/commit/e3ab59557a14e99de42ea72a8f5f2bf96b89a68a))
* **index:** expose revision-bound code-index artifacts and receipt fixture ([1b17bea](https://github.com/morluto/gitcontribute/commit/1b17beadab28020a57fba5be382feb9256cc3b2f))
* **manifest:** export digest-bound contribution evidence ([3854212](https://github.com/morluto/gitcontribute/commit/3854212cc08dbb92cbd02adacef9f9d38397eaab))
* **manifest:** export digest-bound contribution evidence ([5c8cd96](https://github.com/morluto/gitcontribute/commit/5c8cd96801d16ecd1af8ea85373d4c434bf51cc8))
* **manifest:** expose CLI and MCP export workflows ([d6ae1c9](https://github.com/morluto/gitcontribute/commit/d6ae1c962619f20aea49c480411ee01464c6f22c))
* **mcp,codex:** add MCP routing instruction and Codex discovery skill ([293c4fd](https://github.com/morluto/gitcontribute/commit/293c4fd1249fee92ee8962e3944067d9da2618d6))
* **mcp:** add canonical workflow artifact resources ([4bcdb77](https://github.com/morluto/gitcontribute/commit/4bcdb777109821fb5d8933a59969fd8764aa99a3))
* **mcp:** add contribution preflight ([96dba80](https://github.com/morluto/gitcontribute/commit/96dba80ce1494ad9cc8e9ab4a60727b6f7544ff5))
* **mcp:** add contribution preflight for authored pull requests ([2eb11ee](https://github.com/morluto/gitcontribute/commit/2eb11ee9e5a97333151b54c24bf215d1744a7d0e))
* **mcp:** add corpus-backed GitHub search and source artifacts ([43cbd8f](https://github.com/morluto/gitcontribute/commit/43cbd8f134edaa0e0d8767cbf70ab6af6668f96b))
* **mcp:** add corpus-backed GitHub search and source artifacts ([ff6e440](https://github.com/morluto/gitcontribute/commit/ff6e4406245ca9b3db9816dff2651e0238978179))
* **mcp:** add live repository search ([3c49d9c](https://github.com/morluto/gitcontribute/commit/3c49d9c77f39c06717de3174ab51619252e71be0))
* **mcp:** add scalable contribution research workflows ([104d026](https://github.com/morluto/gitcontribute/commit/104d02637376bb4504da2e4f2910a364dcdd5ec0))
* **mcp:** add scalable research and portfolio workflows ([0dd12c6](https://github.com/morluto/gitcontribute/commit/0dd12c6bb67e37ac47f37561581249e7fb546040))
* **mcp:** add typed recovery and coverage workflows ([b37336f](https://github.com/morluto/gitcontribute/commit/b37336f0bce479822040c54f79d79d4baae31f39))
* **mcp:** adopt typed recovery and facet resources ([8b96251](https://github.com/morluto/gitcontribute/commit/8b962512ca1c1472ae739ab0290cc4daf4648713))
* **mcp:** batch repository research and expose recovery guidance ([434fd26](https://github.com/morluto/gitcontribute/commit/434fd26f48a695cbdc4e3c99c7544757cc232f0d))
* **mcp:** bind offline workflows to corpus revisions ([d40f7e9](https://github.com/morluto/gitcontribute/commit/d40f7e91e9ad97e083b2b1d35a7ea830f642f488))
* **mcp:** complete scalable PR health contracts ([3944e77](https://github.com/morluto/gitcontribute/commit/3944e770355b72b384ff73a81f0af030cfc97cfc))
* **mcp:** complete typed recovery and read-only fix-pattern contracts ([3092e8f](https://github.com/morluto/gitcontribute/commit/3092e8f7f5d7065a636352222b940c7fe25db961))
* **mcp:** consolidate repository research reads ([badb5e8](https://github.com/morluto/gitcontribute/commit/badb5e89c60700643eba4a84c6b3bb073aeb7225))
* **mcp:** expose atomic actor tools ([32043b4](https://github.com/morluto/gitcontribute/commit/32043b4e5710df4bf2fa7bff75c1c6822e15f869))
* **mcp:** expose pull-request feedback indexing and search ([3fa4235](https://github.com/morluto/gitcontribute/commit/3fa4235b38c479340218793ed62a5e384396e98b))
* **mcp:** improve agent tool ergonomics ([331ab4e](https://github.com/morluto/gitcontribute/commit/331ab4e2dcb935026a24dbcc05b203cdc26da1da))
* **mcp:** index and search repository pull-request feedback ([02616c8](https://github.com/morluto/gitcontribute/commit/02616c8700cd85d01a67063075dc2c39a71c56ca))
* **mcp:** redesign contribution workflow operations ([76b94fb](https://github.com/morluto/gitcontribute/commit/76b94fbdd913ac39ce1daa5f1e3e1d1c288314ea))
* **onboarding:** add npm setup and release flow ([5ad6c05](https://github.com/morluto/gitcontribute/commit/5ad6c059c1e49f75182f64a58000ddfc70f3e717))
* **onboarding:** add npm setup and release flow ([bd96abd](https://github.com/morluto/gitcontribute/commit/bd96abda9e239bd744317fcf7afa6fccbc38fe7a))
* **preflight:** detect stale contributor forks ([b7adb53](https://github.com/morluto/gitcontribute/commit/b7adb5370fd0ca6a8aa9b15ffd19260c8a448c45))
* **preflight:** detect stale contributor forks ([8f05963](https://github.com/morluto/gitcontribute/commit/8f05963fd5a96920d8b04e448c67f9af573a6bca))
* **radar:** gate candidates on contribution eligibility ([c8aa248](https://github.com/morluto/gitcontribute/commit/c8aa2487e3da0a43d078d87b5ddfb2e078edfa8b))
* **radar:** gate candidates on contribution eligibility ([9d38db8](https://github.com/morluto/gitcontribute/commit/9d38db882a60c264840bdd81b14f408a1ec6e566))
* **radar:** unify related work evidence ([7998117](https://github.com/morluto/gitcontribute/commit/79981177acb0c021aa7393f1288c359393dbb954))
* **radar:** unify related work evidence ([f115e27](https://github.com/morluto/gitcontribute/commit/f115e27f4cbca9671ed85fc891422e72b64012f9))
* redesign contribution workflow operations ([799e1a6](https://github.com/morluto/gitcontribute/commit/799e1a60d344046516c13ab480c67cde5eeadcf6))
* **release:** publish MCP Registry metadata ([3f4300e](https://github.com/morluto/gitcontribute/commit/3f4300e07e914e8cb94e235bce7bb2d84ab7fb32))
* **research:** bind analysis and evidence to provenance ([b73d284](https://github.com/morluto/gitcontribute/commit/b73d2844911695356126a55319bc466629493b1a))
* **setup:** configure detected Devin clients ([fe891fa](https://github.com/morluto/gitcontribute/commit/fe891faa28dc4786945480b489db1ae259b24481))
* **setup:** improve interactive onboarding ([c5dee0e](https://github.com/morluto/gitcontribute/commit/c5dee0e01540206b4794a19f4632affe66cb620a))
* **setup:** make GitContribute discoverable to Codex ([bbf9f1b](https://github.com/morluto/gitcontribute/commit/bbf9f1b7507979758325e49301f03bc88773eb15))
* **setup:** redesign onboarding journey ([cfa4cf3](https://github.com/morluto/gitcontribute/commit/cfa4cf3d72eda0800034bb18f2a186ffe2fc48af))
* start investigations from threads ([#9](https://github.com/morluto/gitcontribute/issues/9)) ([ac1a0fc](https://github.com/morluto/gitcontribute/commit/ac1a0fcd1798bbcc29c4fae6ec67b0b3e26fa8bf))
* **tui:** add contribution research workflow ([5aa5d7e](https://github.com/morluto/gitcontribute/commit/5aa5d7e0b42f1b367814716e6d0096ccd9adbf50))
* **validation:** add repeatable process telemetry ([04d2e72](https://github.com/morluto/gitcontribute/commit/04d2e72c4fb5988f75020d3f55ff2f15b10ef041))
* **validation:** add repeatable runs with process telemetry ([8db0673](https://github.com/morluto/gitcontribute/commit/8db06733ff66d421d7bdf2c79133ca58a52bb79c))
* **validation:** add structured and source-bound evidence workflows ([ca59e8d](https://github.com/morluto/gitcontribute/commit/ca59e8d14cf9f2ef60152250af205a2aaf29656c))
* **validation:** bind runs to workspace snapshots ([f91aaf5](https://github.com/morluto/gitcontribute/commit/f91aaf5d8b7b016cf74079b6eeb38e5c08ef3d0c))
* **validation:** expose repeatable validation workflows ([1b71018](https://github.com/morluto/gitcontribute/commit/1b71018c10b4ed6bc90e6d1882f3a127c2019b2d))
* **validation:** import external execution receipts ([6fc9341](https://github.com/morluto/gitcontribute/commit/6fc9341eff9ab567d1a4cb709517bcb1dd906ddd))
* **validation:** verify expected run observations ([e9181a6](https://github.com/morluto/gitcontribute/commit/e9181a66ec839c6317d4abe0e28392e971e85189))
* **validation:** verify expected run observations ([c34e971](https://github.com/morluto/gitcontribute/commit/c34e9717fd5aa0a25cdb72f1dc4d1ffcd5cbb9b2))
* **workflow:** derive issue contribution dispositions ([c240571](https://github.com/morluto/gitcontribute/commit/c240571839ae8ead4763ac316c59cbe8353b6608))
* **workflow:** prepare evidence from exact issue sets ([c89bc83](https://github.com/morluto/gitcontribute/commit/c89bc83e985311a3cde2ab0a847d7679d7b5c6cc))
* **workflow:** prepare exact issue-set evidence ([ee1f97c](https://github.com/morluto/gitcontribute/commit/ee1f97cb6e600d5ef5d5a5a4d88904de40cc630e))
* **workspace:** adopt existing local worktrees safely ([9faac44](https://github.com/morluto/gitcontribute/commit/9faac44ce2b598fc6b5bf1bad482abf780a58a19))
* **workspace:** expose external worktree adoption ([891f463](https://github.com/morluto/gitcontribute/commit/891f463c5202f8c9e5a1e977d6f2714b07502193))
* **workspace:** model external worktree adoption ([5649f1d](https://github.com/morluto/gitcontribute/commit/5649f1d0199d1b3fac946c647f8014c4fd99bfdc))


### Bug Fixes

* **acquire:** shorten temporary mirror paths ([30b303d](https://github.com/morluto/gitcontribute/commit/30b303d8f60028167f93c441032c116b159c624d))
* **actors:** preserve identity and snapshot integrity ([8bc7ca5](https://github.com/morluto/gitcontribute/commit/8bc7ca559859995e187cbe71707c2caf3baa8585))
* **adapters:** harden external boundary failures ([dca71ee](https://github.com/morluto/gitcontribute/commit/dca71ee11c6ad1dc888c71926df366e354b4405e))
* **app,corpus:** cross-process durable job ownership with SQLite lease ([7b8b3f0](https://github.com/morluto/gitcontribute/commit/7b8b3f0c451d0b478225b5433f75f1270128b618))
* **app:** derive radar duplicate cluster cap from projection metadata ([d980df6](https://github.com/morluto/gitcontribute/commit/d980df64027637c59c7b9efd42bfa800d14bd09a))
* **app:** preserve evidence and provenance errors ([33fa0f4](https://github.com/morluto/gitcontribute/commit/33fa0f4f7b5934fa2d6404309dcdc80513affb74))
* **app:** preserve evidence and provenance errors ([9edcad7](https://github.com/morluto/gitcontribute/commit/9edcad7c752b2df911e8f683d9c076944d618630))
* **app:** prevent heartbeat write bursts ([c2769da](https://github.com/morluto/gitcontribute/commit/c2769da3c4feb1d1d8d96896a95ea7344e3c8389))
* **app:** prevent heartbeat write bursts ([be8967b](https://github.com/morluto/gitcontribute/commit/be8967b86d5406bf338c3012e34cc47f2b0cc23b))
* **app:** remove stale reader import ([848af83](https://github.com/morluto/gitcontribute/commit/848af8331653ac26121d69a367109c8b1c9d973b))
* **app:** report requested thread count in sync batch results ([02c45d0](https://github.com/morluto/gitcontribute/commit/02c45d0e99ce82772d4a7c6757dd0f69bd4cc4ff))
* **app:** validate DeepWiki bounds before provider reads ([e89d788](https://github.com/morluto/gitcontribute/commit/e89d78876ee4e8789d69fc6fb549b5cae75cf326))
* **app:** wire bounded retrieval workflows ([45c51d8](https://github.com/morluto/gitcontribute/commit/45c51d8d01ac4e2a4a66ddba7937282ebe43da44))
* **ci:** harden corpus lifecycle validation ([76fe2df](https://github.com/morluto/gitcontribute/commit/76fe2df6c45bb415a3c10e9f5fb0022d2902184a))
* **ci:** keep setup files bounded and version tests ([fe1fafe](https://github.com/morluto/gitcontribute/commit/fe1fafeec44603f590adc15d8d86ed3a23cccbae))
* **ci:** pass package list to gotestsum ([fd161be](https://github.com/morluto/gitcontribute/commit/fd161becc17a7ecb3bb13397f058b5166277934e))
* **ci:** ratchet existing quality debt ([d7c1a57](https://github.com/morluto/gitcontribute/commit/d7c1a57ef8b0296feafeb249043f895528963134))
* **ci:** satisfy Go lint checks ([ba691d6](https://github.com/morluto/gitcontribute/commit/ba691d68c6cdde7b86f299ee82e3e3435ff20265))
* **ci:** satisfy lint and file size checks ([9b4306a](https://github.com/morluto/gitcontribute/commit/9b4306a7bec8cacb66cd0ae3e3a54d37e10f4b94))
* **ci:** split oversized Go files ([e54d108](https://github.com/morluto/gitcontribute/commit/e54d10888edaf358f110a815753e18c286d37f8c))
* **ci:** split oversized sync files ([3357290](https://github.com/morluto/gitcontribute/commit/33572904125d4a410abbc57f1113921f62c70331))
* **cli,app:** avoid growing oversized Go files ([6c660a0](https://github.com/morluto/gitcontribute/commit/6c660a073267def3211c0757778de11bbd9fb924))
* **cli,service,acquire,mcp:** default-ref and optional workspace input ([74add00](https://github.com/morluto/gitcontribute/commit/74add00f1417ee4fa47dbae4d61e80bbc4e41f21))
* **cli:** guard upgrade confirmation output ([1df35b7](https://github.com/morluto/gitcontribute/commit/1df35b7fa43970ee9a121070bee57c3343c11f64))
* **cli:** guide blocked corpus migration ([be56c9b](https://github.com/morluto/gitcontribute/commit/be56c9b455ffa1023bf8af402a8e4911c13e0e6a))
* **cli:** launch TUI on bare invocation ([6b389da](https://github.com/morluto/gitcontribute/commit/6b389da56c5b01d6b8fb47c7822fd99570c9b041))
* **cli:** validate repository import boundaries ([170b2fe](https://github.com/morluto/gitcontribute/commit/170b2fe604ddd092aa0bd25d7e4895c905c6378d))
* **clusters:** report unchanged projection counts ([d116721](https://github.com/morluto/gitcontribute/commit/d11672194a162cf3ae4f833028b5061368f3a9c4))
* **clusters:** report unchanged projection counts ([aff1260](https://github.com/morluto/gitcontribute/commit/aff1260b10318cb62b4db637eca5fae00c249ef1))
* **config:** accept retired output settings ([1ed08ae](https://github.com/morluto/gitcontribute/commit/1ed08aec86b8ca25c015b9d3c78d76e53ed8f6fa))
* **config:** enforce private configuration directories ([49299bb](https://github.com/morluto/gitcontribute/commit/49299bbbc4500afba840684ed33b0e5e7e822904))
* **config:** migrate retired output settings ([5099e85](https://github.com/morluto/gitcontribute/commit/5099e85fe63d2a852f3e439c2df58d3c6dad0404))
* **context:** bound lifecycle and cleanup work ([b04ecf4](https://github.com/morluto/gitcontribute/commit/b04ecf43532a283d213bafecfe058502a9fde1df))
* **context:** bound lifecycle and cleanup work ([170460d](https://github.com/morluto/gitcontribute/commit/170460da8f4e2abb7d75495a3e35e8170478a303))
* **context:** report reconciliation cleanup errors ([9f646bd](https://github.com/morluto/gitcontribute/commit/9f646bd61953e0f18056e9514ea9013a419655b8))
* **contribution:** satisfy draft validation lint ([9cf46a0](https://github.com/morluto/gitcontribute/commit/9cf46a0d8af97b63190a476f0d6aa21e8fd000f8))
* **control:** make radar bounds explicit ([f00f6a5](https://github.com/morluto/gitcontribute/commit/f00f6a5a2b8575eb1f4eb4a1488b75830b619138))
* **control:** make Radar bounds explicit ([6e925cc](https://github.com/morluto/gitcontribute/commit/6e925ccbb92c13e1152536cf38d11efa98d6f219))
* **corpus:** bind snapshot writes to committed revisions ([fbdf05a](https://github.com/morluto/gitcontribute/commit/fbdf05aef28136f64e9f63ef4f687d4bca48f3cf))
* **corpus:** close portfolio rows before snapshot commit ([1ad6433](https://github.com/morluto/gitcontribute/commit/1ad6433f513673c2a1fa343e8d577482bb4affdc))
* **corpus:** close snapshot reads safely ([35de218](https://github.com/morluto/gitcontribute/commit/35de21821cf86567d7a1a7d2f7d42fe2c3afa722))
* **corpus:** expose precedent history recovery ([d3a8e9d](https://github.com/morluto/gitcontribute/commit/d3a8e9dcb0996daa39d715eb56c14e800cc6806b))
* **corpus:** pass context and handle error in reconcile retry cleanup ([9e66f15](https://github.com/morluto/gitcontribute/commit/9e66f15266c38f3e5ae4c6fed11caa3640a24071))
* **corpus:** preserve authoritative actor projections ([835f4ab](https://github.com/morluto/gitcontribute/commit/835f4abe105a9a99352f0e4f8343c5c16e8b7991))
* **corpus:** preserve connection busy timeout during job reconcile ([a10cdc8](https://github.com/morluto/gitcontribute/commit/a10cdc89449d57a3af8160ca0d254a02ac134f05))
* **corpus:** preserve pull-request feedback coverage ([925874b](https://github.com/morluto/gitcontribute/commit/925874b7d6f3a115c7cc1f1c7047ae7ecbc1a4a1))
* **corpus:** preserve unknown PR merge state ([e75c22b](https://github.com/morluto/gitcontribute/commit/e75c22b674e5836cf1172cb864306bba71871c62))
* **corpus:** preserve unknown PR merge state ([c1a7502](https://github.com/morluto/gitcontribute/commit/c1a7502c3161e11b64b6c03e16fa8052765f171e))
* **corpus:** rely on connection busy_timeout for reconcile BEGIN IMMEDIATE ([46fc8d3](https://github.com/morluto/gitcontribute/commit/46fc8d3cc8cf04a24bd3cf36cd75afa664ea22dc))
* **corpus:** retry BEGIN IMMEDIATE while preserving busy timeout ([8b81861](https://github.com/morluto/gitcontribute/commit/8b81861a13c48503ae56aaadbcd14da14bc35a23))
* **corpus:** retry reconcile lock acquisition ([da42566](https://github.com/morluto/gitcontribute/commit/da42566b14515c659dc4f8999d961c2e74e491cf))
* **corpus:** support Darwin disk stat types ([fbae94d](https://github.com/morluto/gitcontribute/commit/fbae94d53c404f7e2577cc9d1af7de348ea18c5a))
* **corpus:** surface migration and metadata errors ([8f49af6](https://github.com/morluto/gitcontribute/commit/8f49af655ce07da3c186407dcc81e0f3e8386273))
* **corpus:** surface migration and metadata errors ([e98892e](https://github.com/morluto/gitcontribute/commit/e98892e189b7a6b90deee9a5d2ace50b70c7c09f))
* **corpus:** unify ranked retrieval projections ([9e7bfc9](https://github.com/morluto/gitcontribute/commit/9e7bfc9e6ad25658bc5349856d097e10284dd41d))
* critical bug audit - 5 root-cause fixes with regression tests ([092cc99](https://github.com/morluto/gitcontribute/commit/092cc99c62ccb25bfab0e8be045d2446819a29b9))
* **deepwiki:** classify provider error text as unavailable ([1f593bb](https://github.com/morluto/gitcontribute/commit/1f593bb51027d8553cc28bcfb4cb79863dfb6c4f))
* **discovery:** prevent zero-progress split emitting duplicate windows ([cadc77e](https://github.com/morluto/gitcontribute/commit/cadc77e194e3546cd7dfafd454191903c41c2906))
* explain removed compatibility inputs ([14a8856](https://github.com/morluto/gitcontribute/commit/14a88569927496bbe9ea117d0159867a3b53f9e3))
* **feedback:** preserve comment identity and require explicit routing ([ca0b7ca](https://github.com/morluto/gitcontribute/commit/ca0b7ca67d43ddcd9677f47995469f87e6a1b7c9))
* **github:** bind CI snapshots to PR revisions ([2d561e0](https://github.com/morluto/gitcontribute/commit/2d561e0105b175be6b56dec048782f0115911641))
* **github:** capture selected API version provenance ([8a2f9e5](https://github.com/morluto/gitcontribute/commit/8a2f9e5985ccadfa3ca8bd9d4be81e46432e1f1c))
* **github:** preserve transport for CI log downloads ([b9c67bc](https://github.com/morluto/gitcontribute/commit/b9c67bc177d95f51095e3e1299c4c5c58f67c0a8))
* **github:** retry replayable reads safely ([c8e4266](https://github.com/morluto/gitcontribute/commit/c8e42665e381c9e8b28c538ba8dd1efaae60b699))
* **github:** stop fabricating Total in REST feedback pagination ([87a96ad](https://github.com/morluto/gitcontribute/commit/87a96ad88c30d54e64b8501d9f45afbdbab03445))
* harden setup, MCP, and external boundary contracts ([fc020d5](https://github.com/morluto/gitcontribute/commit/fc020d5676e71b475a7d001656ef33bdf07e331f))
* harden validation and adapter boundaries ([2ed471d](https://github.com/morluto/gitcontribute/commit/2ed471d9f0cafef071da8c9919ee105f64cb300e))
* **health:** preserve unknown coverage semantics ([f154640](https://github.com/morluto/gitcontribute/commit/f15464061427b619a01f8ba01b50fe3d8a6bb57b))
* **health:** preserve unknown coverage semantics ([1a6cfef](https://github.com/morluto/gitcontribute/commit/1a6cfef3259549db5147c5b30d14477cd90d41ef))
* **hydration:** refresh exact thread headers ([f152a42](https://github.com/morluto/gitcontribute/commit/f152a42c61a922a2a07229daff6e0ac53066e8d9))
* **jobs:** bound concurrent execution ([180128e](https://github.com/morluto/gitcontribute/commit/180128e12fb7797abc1a87c01b5e26183a15c869))
* **jobs:** surface transition and batch errors ([8028122](https://github.com/morluto/gitcontribute/commit/8028122ec700f8aa7f6747f41d8392b47fe159a8))
* **jobs:** surface transition and batch errors ([8034d87](https://github.com/morluto/gitcontribute/commit/8034d87584635b63723ed036ee64151f1116f293))
* **lint:** remove stale workspace resolver ([45e6043](https://github.com/morluto/gitcontribute/commit/45e60435d7afd674245b7402fc89d7ec45be896b))
* **mcp:** accept exact thread sync selectors ([9b275e6](https://github.com/morluto/gitcontribute/commit/9b275e6f14d934f6251a801db7405494f70f0cbb))
* **mcp:** accept exact thread sync selectors ([447ea29](https://github.com/morluto/gitcontribute/commit/447ea29d0f46df9946282d48bc25ed8d1e472c3d))
* **mcp:** add typed recovery for incomplete reads ([3823689](https://github.com/morluto/gitcontribute/commit/3823689443949f6857e59ba74580df699f761149))
* **mcp:** avoid recovery encoding overflow ([1955ee1](https://github.com/morluto/gitcontribute/commit/1955ee1f28e0cf0b5ba031a608e7bbc30339f3a4))
* **mcp:** complete revision-bound workflow handoffs ([5f83881](https://github.com/morluto/gitcontribute/commit/5f83881b0f0ddc6e5398c9915fce34f36d671b59))
* **mcp:** disambiguate sync and hydration routing ([a4d6ef4](https://github.com/morluto/gitcontribute/commit/a4d6ef436284acbfbe0bf55fa7beb657568d5286))
* **mcp:** enforce bounded scalable operations ([e3213ca](https://github.com/morluto/gitcontribute/commit/e3213ca5ee2e58195e1b86afe601218956b53c6e))
* **mcp:** enforce workflow input boundaries ([faebca9](https://github.com/morluto/gitcontribute/commit/faebca9fd2f7150b9e61b3f464256e5a98a6db0f))
* **mcp:** expose executable coverage recovery ([4f82564](https://github.com/morluto/gitcontribute/commit/4f82564978429b4682cc4e67f703ee832a07737d))
* **mcp:** keep private runtime registration aligned ([1556cbe](https://github.com/morluto/gitcontribute/commit/1556cbe552303fde37a91a920a8c5c5c43b3416c))
* **mcp:** preserve bounded portfolio handoffs ([105c7f7](https://github.com/morluto/gitcontribute/commit/105c7f7b41e1b59b301bbc8d0336a61f35753e40))
* **mcp:** provide typed recovery for incomplete reads ([b6e6b78](https://github.com/morluto/gitcontribute/commit/b6e6b78e77a946c2a367708698c92cb8f6dbfa45))
* **mcp:** remove obsolete facet aliases ([f883f40](https://github.com/morluto/gitcontribute/commit/f883f409a9a0f14f1815aa7e290543a79901fc52))
* **mcp:** return actionable validation errors ([5da4188](https://github.com/morluto/gitcontribute/commit/5da41880837614f3f4a1bdbd9e7c03ef31c01979))
* **mcp:** return schema registration errors ([8f863f6](https://github.com/morluto/gitcontribute/commit/8f863f6cacb7f17c430b5d1369b34c9f71e0fac4))
* **mcp:** return schema registration errors ([b9852d0](https://github.com/morluto/gitcontribute/commit/b9852d0f09f3b8b19465235246cdf22ea4b9087d))
* **mcp:** satisfy lint in recovery and facet paths ([20e4add](https://github.com/morluto/gitcontribute/commit/20e4add906d6476b30399c7ebf3fc478457eaebe))
* **mcp:** self-seed exact pull request feedback sync ([281ec17](https://github.com/morluto/gitcontribute/commit/281ec1747e7d5cd429d00f0ac289483a3d06bb72))
* **mcp:** synchronize catalog and feedback routing ([2ed99ea](https://github.com/morluto/gitcontribute/commit/2ed99ea8e45568311d8232218d1bfdd0263d397a))
* **metadata:** inspect existing corpus schema ([2f3bd53](https://github.com/morluto/gitcontribute/commit/2f3bd53392baa255cc0e389b81eac7669478fcb1))
* **metadata:** inspect existing corpus schema ([d95f5e4](https://github.com/morluto/gitcontribute/commit/d95f5e41d67dd6afe6b39d37646ebdd54993886e))
* **npm:** mark launcher executable ([f3bed35](https://github.com/morluto/gitcontribute/commit/f3bed3559533c1827e35fbec2ca441f23fb65b52))
* **platform:** preserve atomic replacement and process ownership ([b4abd3b](https://github.com/morluto/gitcontribute/commit/b4abd3bdbe8c17bfd68fa3938ec96e72e6c1be0c))
* propagate MCP job progress errors ([729595d](https://github.com/morluto/gitcontribute/commit/729595defb033fc2a812e350220ff38a93c4162c))
* **radar:** align CLI with bounded population ([c4ad6cb](https://github.com/morluto/gitcontribute/commit/c4ad6cbc1bababfb56dc94fcc50d0746c5dc1c0d))
* **radar:** align CLI with bounded population ([d3e042e](https://github.com/morluto/gitcontribute/commit/d3e042e9c6412313cd08247935cd799c8fd1a102))
* **release:** allow manual tag publishing ([96e8162](https://github.com/morluto/gitcontribute/commit/96e8162cbb7a59f34b9fd75eedc75802585d4472))
* **release:** allow manual tag publishing ([92d6418](https://github.com/morluto/gitcontribute/commit/92d6418ef7d2b21ded6434be6b401a13309d49ca))
* **release:** manage npm version in release PRs ([db3a3fe](https://github.com/morluto/gitcontribute/commit/db3a3fe4a72a079a3fcc3a895a74522d448f8e76))
* **release:** pass NODE_AUTH_TOKEN to npm publish step ([cfba588](https://github.com/morluto/gitcontribute/commit/cfba5881c600bed213a46679052635c63995f67a))
* **release:** resolve npm smoke tarball path ([6e98d13](https://github.com/morluto/gitcontribute/commit/6e98d1362cb28f3b5ba3895db9b1b20b1e4237a1))
* **release:** skip existing assets during tag recovery ([bdb6e23](https://github.com/morluto/gitcontribute/commit/bdb6e233187a2789fe007367978c6c745d331346))
* **release:** skip existing assets during tag recovery ([64902a2](https://github.com/morluto/gitcontribute/commit/64902a2147b991be8586d2bdaf49601197a0f774))
* **release:** update secure toolchain baseline ([41acdbd](https://github.com/morluto/gitcontribute/commit/41acdbd0d73e4290b272d3f4aa10106ca784fde3))
* **release:** use npm trusted publishing ([73e6348](https://github.com/morluto/gitcontribute/commit/73e634859b18d9c0ec26fddcb66f9b81ada3ac6c))
* **release:** use npm trusted publishing ([b32a535](https://github.com/morluto/gitcontribute/commit/b32a5357c2b83ec8d5a938505c7e4d02e94f8269))
* **release:** use trusted-publishing npm client ([5a40b9e](https://github.com/morluto/gitcontribute/commit/5a40b9e638132f3e529126d73fba1aa58db24662))
* retry reconciliation during SQLite contention ([8e1c876](https://github.com/morluto/gitcontribute/commit/8e1c87655febcd6fae9a08dc58fe905e313ee6ec))
* satisfy shutdown lint check ([5ffd021](https://github.com/morluto/gitcontribute/commit/5ffd0213137095750510e667d557a7381fcf08a2))
* **search:** align corpus ranking and MCP retrieval contracts ([900a55e](https://github.com/morluto/gitcontribute/commit/900a55e7a6eb4bfc31070a0b99a9719eefc87de1))
* **search:** index hydrated discussion evidence ([3ef4ed3](https://github.com/morluto/gitcontribute/commit/3ef4ed30242ca29d16930ab1161742840f9b53d1))
* **search:** index hydrated discussion evidence ([d6c4da2](https://github.com/morluto/gitcontribute/commit/d6c4da26e14183ba414e7164feac898ed8ec52eb))
* **search:** keep explanations aligned with indexed evidence ([0335329](https://github.com/morluto/gitcontribute/commit/0335329d795aee5718b16af7c3ac935df6b5a388))
* **search:** preserve query and retry contracts ([ceece44](https://github.com/morluto/gitcontribute/commit/ceece44da14d5db9d9ee01147a25b36109c8de35))
* **search:** report unknown merge-state matches ([a5f6509](https://github.com/morluto/gitcontribute/commit/a5f65097b1ea1a993064aa7fbcf5531c7edb2f2b))
* **search:** surface unknown merge-state matches ([7577bae](https://github.com/morluto/gitcontribute/commit/7577baeee2c3d5a788371d9b1ca0a2d631188faa))
* **security:** constrain GitHub URL matches ([615c625](https://github.com/morluto/gitcontribute/commit/615c625767b0bc041599918263e76f955992ff43))
* **security:** reject credential-bearing git remotes ([77fe530](https://github.com/morluto/gitcontribute/commit/77fe53017af55049911de7f1e86ce08cc6a56ef6))
* **security:** reject credential-bearing Git remotes ([1d1484c](https://github.com/morluto/gitcontribute/commit/1d1484c573a97b9c5e9f8518141805fd2db9aa55))
* **seeds:** separate outcomes from issue context ([cb0044c](https://github.com/morluto/gitcontribute/commit/cb0044c8742e736ad4caaf5c899f6bb8730c1b4c))
* **seeds:** separate outcomes from issue context ([cd6cddf](https://github.com/morluto/gitcontribute/commit/cd6cddf1d391fbaca5687ff55dd645d8340c9d0c))
* **setup:** default final consent to apply ([996346e](https://github.com/morluto/gitcontribute/commit/996346e4d0629acb2eb156c4d4f44fc02d6c4dde))
* **setup:** edit Codex configuration structurally ([2a9a1ff](https://github.com/morluto/gitcontribute/commit/2a9a1ff3b1dae7355517a799cc729aae4101c5a5))
* **setup:** honor redirected Devin config roots ([5a9241a](https://github.com/morluto/gitcontribute/commit/5a9241a7b142abceb8ba6c3af14de1e6fa43a5e6))
* **setup:** keep local verification environment-independent ([817d847](https://github.com/morluto/gitcontribute/commit/817d84767d22293194a37b79b11b17b3d1bd81c0))
* **setup:** report failed verification checks ([b377730](https://github.com/morluto/gitcontribute/commit/b37773002824b6f1260bbe2ecef0ed3a835dd067))
* **setup:** resolve Devin config paths by platform ([2908971](https://github.com/morluto/gitcontribute/commit/2908971b1b9429e0732e5005e4bb310acba29693))
* **setup:** restore registrations on Windows ([4c2060b](https://github.com/morluto/gitcontribute/commit/4c2060bd11fd8470f8e06db31b18186296a6e79b))
* **setup:** satisfy lint and preserve dry-run output ([7de5d16](https://github.com/morluto/gitcontribute/commit/7de5d163e3d49d8aae954a8feeac48ebb48b23ca))
* **setup:** surface corpus blockers before side effects ([dc943e0](https://github.com/morluto/gitcontribute/commit/dc943e04deffe1195035f144bd1f624f85cfd2b6))
* **setup:** use direct npx package launcher ([3fda85e](https://github.com/morluto/gitcontribute/commit/3fda85e366872dcb10640448feaf6933adf714d1))
* **setup:** verify applied installation state ([24e976e](https://github.com/morluto/gitcontribute/commit/24e976edf86edc2e538b3737c35312bcaf385225))
* **sync:** bound GitHub request fanout ([5ee0ef0](https://github.com/morluto/gitcontribute/commit/5ee0ef0d44513b7f20e6d0ae68749891e29a17fd))
* **sync:** bound GitHub request fanout ([7d60e04](https://github.com/morluto/gitcontribute/commit/7d60e042a3257cc1444ebba33c424a646ebb8059))
* **sync:** show request plan before reads ([a891645](https://github.com/morluto/gitcontribute/commit/a891645b5a7863523185f93c939ebca895867ac1))
* **sync:** show request plan before reads ([6321c20](https://github.com/morluto/gitcontribute/commit/6321c208abed71bec60e569353545ca9fb8b7715))
* **test:** close batch sync services ([7e1ba9d](https://github.com/morluto/gitcontribute/commit/7e1ba9d41b0dddc15496d5f0bf4dfd1ca9500a8b))
* **test:** make platform assertions portable ([0bd2230](https://github.com/morluto/gitcontribute/commit/0bd2230304d363f56f1d1eca02016ee6cbfd1ce7))
* **tests:** compare canonical worktree paths ([e98ccbd](https://github.com/morluto/gitcontribute/commit/e98ccbd40c36518a0bea4583ce0a767bf5175a80))
* tighten snapshot token boundaries and eval timing ([4b4c45d](https://github.com/morluto/gitcontribute/commit/4b4c45d35e8a169c0d06ed7ceda3e60f2884aee1))
* **upgrade:** clarify that the upgrade takes effect after an MCP client restart ([f4cdef2](https://github.com/morluto/gitcontribute/commit/f4cdef26fbef7994c7a6c1462bd37596b77bb177))
* **upgrade:** guard empty npm command arguments ([3b5104f](https://github.com/morluto/gitcontribute/commit/3b5104f1785b5ee3c1cba14962a57517f555dd64))
* **upgrade:** retain npx executable for activation ([39218f7](https://github.com/morluto/gitcontribute/commit/39218f79b0e1bae6ff63ba62acd9894e52a1db99))
* **workflows:** close review race and batch job gaps ([cd08162](https://github.com/morluto/gitcontribute/commit/cd08162b9d9c66fc27c66829393b7882bb467c8e))
* **workflows:** preserve complete PR snapshots ([fbf0e02](https://github.com/morluto/gitcontribute/commit/fbf0e022c0ee1e167c37278ba153ac51d52720cb))
* **workflows:** preserve effective batch semantics ([3408267](https://github.com/morluto/gitcontribute/commit/3408267a6de3d6e134139130656957adc6ed7962))
* **workspace:** clean up leaked worktree on concurrent Create race ([dd5661c](https://github.com/morluto/gitcontribute/commit/dd5661ccf9bd0566bd6eec6ecd609a1ca352b39d))
* **workspace:** preserve concurrent worktrees and merge conflicts ([531756d](https://github.com/morluto/gitcontribute/commit/531756d6c2d8c981e4ef49bb0ae1b6effabf2539))
* **workspace:** propagate context cancellation in git resolution paths ([b183851](https://github.com/morluto/gitcontribute/commit/b183851a8820421aa468840ebc2e51f58677f2f6))
* **workspace:** remove false-positive merge conflict from file content ([6682278](https://github.com/morluto/gitcontribute/commit/6682278b038df103800fd05113d6f8a9d18ea7e1))
* **workspace:** resolve omitted refs from repository defaults ([615e94f](https://github.com/morluto/gitcontribute/commit/615e94fe0e67387fa8c45b911cb38fd5e4c066ca))


### Performance Improvements

* bound indexing, jobs, search, and clustering ([9c96943](https://github.com/morluto/gitcontribute/commit/9c96943dd07fb32efc2b8a7ea8791444e3f2ddc2))
* **clustering:** prune impossible duplicate pairs ([047d6a7](https://github.com/morluto/gitcontribute/commit/047d6a72888c1c6eae8ea20e3f68f5b77b88d620))
* **codeindex:** batch blob reads and reuse snapshots ([d78acd7](https://github.com/morluto/gitcontribute/commit/d78acd71a82a8f308097db6e16d8b2661a79d374))
* **jobs:** bound execution and batch cancellations ([c70dc60](https://github.com/morluto/gitcontribute/commit/c70dc60e1bcb9b03596cf59db7d76113519bc279))
* **mcp:** reduce payloads and batch corpus reads ([eaa944a](https://github.com/morluto/gitcontribute/commit/eaa944a13cf31a9ffd185fbcd9be47e30cd4d259))
* **search:** share one snapshot for pages and counts ([ce1fea8](https://github.com/morluto/gitcontribute/commit/ce1fea8db1a646236699ed7d77c5018c40250233))

## [2.0.0] (2026-08-09)

### ⚠ BREAKING CHANGES

* **mcp:** `github.search_threads` and `github.read_source_files` now require
  `repository: {owner, repo}`. Flat `owner` and `repo` request fields are no
  longer accepted. See [the v2 MCP migration guide](docs/mcp-v2-migration.md).

### Features

* **mcp:** add optional repository scope to authored pull-request portfolios.

### Bug Fixes

* **mcp:** return host-neutral native resource links for durable artifacts.

## [1.1.0](https://github.com/morluto/gitcontribute/compare/v1.0.0...v1.1.0) (2026-08-08)


### Features

* **preflight:** detect stale contributor forks ([b7adb53](https://github.com/morluto/gitcontribute/commit/b7adb5370fd0ca6a8aa9b15ffd19260c8a448c45))
* **preflight:** detect stale contributor forks ([8f05963](https://github.com/morluto/gitcontribute/commit/8f05963fd5a96920d8b04e448c67f9af573a6bca))
* **release:** publish MCP Registry metadata ([3f4300e](https://github.com/morluto/gitcontribute/commit/3f4300e07e914e8cb94e235bce7bb2d84ab7fb32))
* **validation:** add structured and source-bound evidence workflows ([ca59e8d](https://github.com/morluto/gitcontribute/commit/ca59e8d14cf9f2ef60152250af205a2aaf29656c))


### Bug Fixes

* **cli:** guard upgrade confirmation output ([1df35b7](https://github.com/morluto/gitcontribute/commit/1df35b7fa43970ee9a121070bee57c3343c11f64))
* critical bug audit - 5 root-cause fixes with regression tests ([092cc99](https://github.com/morluto/gitcontribute/commit/092cc99c62ccb25bfab0e8be045d2446819a29b9))
* **discovery:** prevent zero-progress split emitting duplicate windows ([cadc77e](https://github.com/morluto/gitcontribute/commit/cadc77e194e3546cd7dfafd454191903c41c2906))
* **github:** stop fabricating Total in REST feedback pagination ([87a96ad](https://github.com/morluto/gitcontribute/commit/87a96ad88c30d54e64b8501d9f45afbdbab03445))
* **mcp:** preserve bounded portfolio handoffs ([105c7f7](https://github.com/morluto/gitcontribute/commit/105c7f7b41e1b59b301bbc8d0336a61f35753e40))
* **workspace:** clean up leaked worktree on concurrent Create race ([dd5661c](https://github.com/morluto/gitcontribute/commit/dd5661ccf9bd0566bd6eec6ecd609a1ca352b39d))
* **workspace:** preserve concurrent worktrees and merge conflicts ([531756d](https://github.com/morluto/gitcontribute/commit/531756d6c2d8c981e4ef49bb0ae1b6effabf2539))
* **workspace:** propagate context cancellation in git resolution paths ([b183851](https://github.com/morluto/gitcontribute/commit/b183851a8820421aa468840ebc2e51f58677f2f6))
* **workspace:** remove false-positive merge conflict from file content ([6682278](https://github.com/morluto/gitcontribute/commit/6682278b038df103800fd05113d6f8a9d18ea7e1))

## [1.0.0](https://github.com/morluto/gitcontribute/compare/v0.19.0...v1.0.0) (2026-08-03)


### ⚠ BREAKING CHANGES

* **mcp:** corpus.search_code_batch is now corpus.search_code, corpus.list_pull_requests is now corpus.search_pull_requests, github.sync_ci_failures is now github.sync_pull_request_ci, and prescriptive composite workflow tools are no longer advertised.

### Features

* **corpus:** add first-class actor projections ([b8441f3](https://github.com/morluto/gitcontribute/commit/b8441f3ee4d04b9dfd455137489661123a5a6a08))
* **github:** add bounded actor acquisition adapters ([e3ab595](https://github.com/morluto/gitcontribute/commit/e3ab59557a14e99de42ea72a8f5f2bf96b89a68a))
* **mcp:** expose atomic actor tools ([32043b4](https://github.com/morluto/gitcontribute/commit/32043b4e5710df4bf2fa7bff75c1c6822e15f869))


### Bug Fixes

* **actors:** preserve identity and snapshot integrity ([8bc7ca5](https://github.com/morluto/gitcontribute/commit/8bc7ca559859995e187cbe71707c2caf3baa8585))
* **corpus:** preserve authoritative actor projections ([835f4ab](https://github.com/morluto/gitcontribute/commit/835f4abe105a9a99352f0e4f8343c5c16e8b7991))
* **feedback:** preserve comment identity and require explicit routing ([ca0b7ca](https://github.com/morluto/gitcontribute/commit/ca0b7ca67d43ddcd9677f47995469f87e6a1b7c9))
* **mcp:** keep private runtime registration aligned ([1556cbe](https://github.com/morluto/gitcontribute/commit/1556cbe552303fde37a91a920a8c5c5c43b3416c))
* **mcp:** synchronize catalog and feedback routing ([2ed99ea](https://github.com/morluto/gitcontribute/commit/2ed99ea8e45568311d8232218d1bfdd0263d397a))

## [0.19.0](https://github.com/morluto/gitcontribute/compare/v0.18.0...v0.19.0) (2026-08-01)


### Features

* **corpus:** add pull-request feedback projections ([d1a5c10](https://github.com/morluto/gitcontribute/commit/d1a5c109f26e2c812ceab9ac426d31359219cadf))
* **mcp:** add contribution preflight ([96dba80](https://github.com/morluto/gitcontribute/commit/96dba80ce1494ad9cc8e9ab4a60727b6f7544ff5))
* **mcp:** add contribution preflight for authored pull requests ([2eb11ee](https://github.com/morluto/gitcontribute/commit/2eb11ee9e5a97333151b54c24bf215d1744a7d0e))
* **mcp:** expose pull-request feedback indexing and search ([3fa4235](https://github.com/morluto/gitcontribute/commit/3fa4235b38c479340218793ed62a5e384396e98b))
* **mcp:** index and search repository pull-request feedback ([02616c8](https://github.com/morluto/gitcontribute/commit/02616c8700cd85d01a67063075dc2c39a71c56ca))


### Bug Fixes

* **corpus:** expose precedent history recovery ([d3a8e9d](https://github.com/morluto/gitcontribute/commit/d3a8e9dcb0996daa39d715eb56c14e800cc6806b))
* **corpus:** preserve pull-request feedback coverage ([925874b](https://github.com/morluto/gitcontribute/commit/925874b7d6f3a115c7cc1f1c7047ae7ecbc1a4a1))
* **mcp:** add typed recovery for incomplete reads ([3823689](https://github.com/morluto/gitcontribute/commit/3823689443949f6857e59ba74580df699f761149))
* **mcp:** provide typed recovery for incomplete reads ([b6e6b78](https://github.com/morluto/gitcontribute/commit/b6e6b78e77a946c2a367708698c92cb8f6dbfa45))
* **tests:** compare canonical worktree paths ([e98ccbd](https://github.com/morluto/gitcontribute/commit/e98ccbd40c36518a0bea4583ce0a767bf5175a80))
* **upgrade:** retain npx executable for activation ([39218f7](https://github.com/morluto/gitcontribute/commit/39218f79b0e1bae6ff63ba62acd9894e52a1db99))

## [0.18.0](https://github.com/morluto/gitcontribute/compare/v0.17.0...v0.18.0) (2026-08-01)


### Features

* **mcp:** add corpus-backed GitHub search and source artifacts ([43cbd8f](https://github.com/morluto/gitcontribute/commit/43cbd8f134edaa0e0d8767cbf70ab6af6668f96b))
* **mcp:** add corpus-backed GitHub search and source artifacts ([ff6e440](https://github.com/morluto/gitcontribute/commit/ff6e4406245ca9b3db9816dff2651e0238978179))


### Bug Fixes

* **upgrade:** guard empty npm command arguments ([3b5104f](https://github.com/morluto/gitcontribute/commit/3b5104f1785b5ee3c1cba14962a57517f555dd64))

## [0.17.0](https://github.com/morluto/gitcontribute/compare/v0.16.0...v0.17.0) (2026-07-31)


### Features

* **corpus:** bind offline reads to corpus revisions ([6e75235](https://github.com/morluto/gitcontribute/commit/6e7523533a213ae7dbc4827bb36caaf97f3c9136))
* **corpus:** persist immutable snapshot and index identities ([d1f41e0](https://github.com/morluto/gitcontribute/commit/d1f41e0aedff074bdb6ea290ccebfc1283ae666b))
* **index:** expose revision-bound code-index artifacts and receipt fixture ([1b17bea](https://github.com/morluto/gitcontribute/commit/1b17beadab28020a57fba5be382feb9256cc3b2f))
* **mcp:** add typed recovery and coverage workflows ([b37336f](https://github.com/morluto/gitcontribute/commit/b37336f0bce479822040c54f79d79d4baae31f39))
* **mcp:** adopt typed recovery and facet resources ([8b96251](https://github.com/morluto/gitcontribute/commit/8b962512ca1c1472ae739ab0290cc4daf4648713))
* **mcp:** bind offline workflows to corpus revisions ([d40f7e9](https://github.com/morluto/gitcontribute/commit/d40f7e91e9ad97e083b2b1d35a7ea830f642f488))
* **mcp:** complete typed recovery and read-only fix-pattern contracts ([3092e8f](https://github.com/morluto/gitcontribute/commit/3092e8f7f5d7065a636352222b940c7fe25db961))
* **research:** bind analysis and evidence to provenance ([b73d284](https://github.com/morluto/gitcontribute/commit/b73d2844911695356126a55319bc466629493b1a))


### Bug Fixes

* **app:** remove stale reader import ([848af83](https://github.com/morluto/gitcontribute/commit/848af8331653ac26121d69a367109c8b1c9d973b))
* **cli:** validate repository import boundaries ([170b2fe](https://github.com/morluto/gitcontribute/commit/170b2fe604ddd092aa0bd25d7e4895c905c6378d))
* **config:** enforce private configuration directories ([49299bb](https://github.com/morluto/gitcontribute/commit/49299bbbc4500afba840684ed33b0e5e7e822904))
* **corpus:** bind snapshot writes to committed revisions ([fbdf05a](https://github.com/morluto/gitcontribute/commit/fbdf05aef28136f64e9f63ef4f687d4bca48f3cf))
* **mcp:** complete revision-bound workflow handoffs ([5f83881](https://github.com/morluto/gitcontribute/commit/5f83881b0f0ddc6e5398c9915fce34f36d671b59))
* **mcp:** expose executable coverage recovery ([4f82564](https://github.com/morluto/gitcontribute/commit/4f82564978429b4682cc4e67f703ee832a07737d))
* **mcp:** satisfy lint in recovery and facet paths ([20e4add](https://github.com/morluto/gitcontribute/commit/20e4add906d6476b30399c7ebf3fc478457eaebe))
* **mcp:** self-seed exact pull request feedback sync ([281ec17](https://github.com/morluto/gitcontribute/commit/281ec1747e7d5cd429d00f0ac289483a3d06bb72))
* **release:** allow manual tag publishing ([96e8162](https://github.com/morluto/gitcontribute/commit/96e8162cbb7a59f34b9fd75eedc75802585d4472))
* **release:** allow manual tag publishing ([92d6418](https://github.com/morluto/gitcontribute/commit/92d6418ef7d2b21ded6434be6b401a13309d49ca))
* **release:** skip existing assets during tag recovery ([bdb6e23](https://github.com/morluto/gitcontribute/commit/bdb6e233187a2789fe007367978c6c745d331346))
* **release:** skip existing assets during tag recovery ([64902a2](https://github.com/morluto/gitcontribute/commit/64902a2147b991be8586d2bdaf49601197a0f774))
* **release:** use npm trusted publishing ([73e6348](https://github.com/morluto/gitcontribute/commit/73e634859b18d9c0ec26fddcb66f9b81ada3ac6c))
* **release:** use npm trusted publishing ([b32a535](https://github.com/morluto/gitcontribute/commit/b32a5357c2b83ec8d5a938505c7e4d02e94f8269))
* tighten snapshot token boundaries and eval timing ([4b4c45d](https://github.com/morluto/gitcontribute/commit/4b4c45d35e8a169c0d06ed7ceda3e60f2884aee1))

## [0.16.0](https://github.com/morluto/gitcontribute/compare/v0.15.0...v0.16.0) (2026-07-30)


### Features

* **mcp:** redesign contribution workflow operations ([76b94fb](https://github.com/morluto/gitcontribute/commit/76b94fbdd913ac39ce1daa5f1e3e1d1c288314ea))
* redesign contribution workflow operations ([799e1a6](https://github.com/morluto/gitcontribute/commit/799e1a60d344046516c13ab480c67cde5eeadcf6))
* **setup:** configure detected Devin clients ([fe891fa](https://github.com/morluto/gitcontribute/commit/fe891faa28dc4786945480b489db1ae259b24481))


### Bug Fixes

* **app:** prevent heartbeat write bursts ([c2769da](https://github.com/morluto/gitcontribute/commit/c2769da3c4feb1d1d8d96896a95ea7344e3c8389))
* **app:** prevent heartbeat write bursts ([be8967b](https://github.com/morluto/gitcontribute/commit/be8967b86d5406bf338c3012e34cc47f2b0cc23b))
* **github:** bind CI snapshots to PR revisions ([2d561e0](https://github.com/morluto/gitcontribute/commit/2d561e0105b175be6b56dec048782f0115911641))
* **github:** preserve transport for CI log downloads ([b9c67bc](https://github.com/morluto/gitcontribute/commit/b9c67bc177d95f51095e3e1299c4c5c58f67c0a8))
* **mcp:** enforce workflow input boundaries ([faebca9](https://github.com/morluto/gitcontribute/commit/faebca9fd2f7150b9e61b3f464256e5a98a6db0f))
* **release:** pass NODE_AUTH_TOKEN to npm publish step ([cfba588](https://github.com/morluto/gitcontribute/commit/cfba5881c600bed213a46679052635c63995f67a))
* **setup:** honor redirected Devin config roots ([5a9241a](https://github.com/morluto/gitcontribute/commit/5a9241a7b142abceb8ba6c3af14de1e6fa43a5e6))
* **setup:** resolve Devin config paths by platform ([2908971](https://github.com/morluto/gitcontribute/commit/2908971b1b9429e0732e5005e4bb310acba29693))
* **upgrade:** clarify that the upgrade takes effect after an MCP client restart ([f4cdef2](https://github.com/morluto/gitcontribute/commit/f4cdef26fbef7994c7a6c1462bd37596b77bb177))
* **workflows:** close review race and batch job gaps ([cd08162](https://github.com/morluto/gitcontribute/commit/cd08162b9d9c66fc27c66829393b7882bb467c8e))
* **workflows:** preserve complete PR snapshots ([fbf0e02](https://github.com/morluto/gitcontribute/commit/fbf0e022c0ee1e167c37278ba153ac51d52720cb))
* **workflows:** preserve effective batch semantics ([3408267](https://github.com/morluto/gitcontribute/commit/3408267a6de3d6e134139130656957adc6ed7962))

## [0.15.0](https://github.com/morluto/gitcontribute/compare/v0.14.0...v0.15.0) (2026-07-29)


### Features

* **mcp:** add canonical workflow artifact resources ([4bcdb77](https://github.com/morluto/gitcontribute/commit/4bcdb777109821fb5d8933a59969fd8764aa99a3))

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
