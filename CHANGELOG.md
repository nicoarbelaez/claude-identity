# Changelog

## [0.7.0](https://github.com/nicoarbelaez/claude-identity/compare/v0.6.0...v0.7.0) (2026-10-27)


### Features

* uso por cuenta en /account, cid statusline y cambio de cuenta seguro ([#31](https://github.com/nicoarbelaez/claude-identity/issues/31)) ([8319a25](https://github.com/nicoarbelaez/claude-identity/commit/8319a2577093e741c3563308e9da71419583d44b))

## [0.6.0](https://github.com/nicoarbelaez/claude-identity/compare/v0.5.0...v0.6.0) (2026-10-26)


### Features

* cid update y aviso de versión nueva en cid version ([#29](https://github.com/nicoarbelaez/claude-identity/issues/29)) ([dc6aa0c](https://github.com/nicoarbelaez/claude-identity/commit/dc6aa0c062fb54ab2a0b24e5ec953893464ae937))

## [0.5.0](https://github.com/nicoarbelaez/claude-identity/compare/v0.4.0...v0.5.0) (2026-10-26)


### Features

* **cli:** cid config get, set y unset con validación y recuperación de archivos dañados ([#28](https://github.com/nicoarbelaez/claude-identity/issues/28)) ([51d8697](https://github.com/nicoarbelaez/claude-identity/commit/51d8697b4f4054a94b7df68d7b861ed82e5283d7))
* **hook:** /account add y relogin inician el login desde Claude Code ([#26](https://github.com/nicoarbelaez/claude-identity/issues/26)) ([1943735](https://github.com/nicoarbelaez/claude-identity/commit/1943735f19d2cad4e1da9a5d7064500a5938dff1))

## [0.4.0](https://github.com/nicoarbelaez/claude-identity/compare/v0.3.1...v0.4.0) (2026-10-26)


### Features

* **cli:** agregar cid uninstall para eliminar perfiles y configuración ([#23](https://github.com/nicoarbelaez/claude-identity/issues/23)) ([ec600a9](https://github.com/nicoarbelaez/claude-identity/commit/ec600a9e05e8d0ec30cf1fe79627054e23f080d9))
* **cli:** cid add y cid relogin inician el login de Claude Code automáticamente ([#25](https://github.com/nicoarbelaez/claude-identity/issues/25)) ([751e7ea](https://github.com/nicoarbelaez/claude-identity/commit/751e7ea865fa4ed8fcc0d479f5390c52933ba7ee))

## [0.3.1](https://github.com/nicoarbelaez/claude-identity/compare/v0.3.0...v0.3.1) (2026-10-26)


### Bug Fixes

* **credentials:** reportar estado desconocido para credenciales vacías o null ([#21](https://github.com/nicoarbelaez/claude-identity/issues/21)) ([f095d9e](https://github.com/nicoarbelaez/claude-identity/commit/f095d9e45b08153d5bc82dad5176c3e3173f9bd6))

## [0.3.0](https://github.com/nicoarbelaez/claude-identity/compare/v0.2.0...v0.3.0) (2026-10-26)


### Features

* estado del token por perfil y cid relogin ([#19](https://github.com/nicoarbelaez/claude-identity/issues/19)) ([3b01f81](https://github.com/nicoarbelaez/claude-identity/commit/3b01f81c332f6b3b68dbab02cb5d7ae8576debcb))
* **plugin:** /account instantáneo con hook y skill de respaldo ([#15](https://github.com/nicoarbelaez/claude-identity/issues/15)) ([c6700bc](https://github.com/nicoarbelaez/claude-identity/commit/c6700bca774a0780f623e879b5eb246f4e634f8f))


### Bug Fixes

* **add:** bloquear sobrescritura y duplicado de cuentas en cid add ([#17](https://github.com/nicoarbelaez/claude-identity/issues/17)) ([d955eae](https://github.com/nicoarbelaez/claude-identity/commit/d955eaee00642024f7d74872cfb9788778d0d0e2))
* fallar en cerrado al eliminar perfil y propagar errores de Active ([#14](https://github.com/nicoarbelaez/claude-identity/issues/14)) ([7fff54d](https://github.com/nicoarbelaez/claude-identity/commit/7fff54d2f1b90735da2753b5a1c7df16ad1d61f0))
* **install:** agregar cid al PATH y mejorar la guia de instalacion ([#18](https://github.com/nicoarbelaez/claude-identity/issues/18)) ([1efb652](https://github.com/nicoarbelaez/claude-identity/commit/1efb652af7d67ffe7da16f9425224895e9f907a3))
* **install:** no reportar fallo de PATH si solo falla el aviso de entorno ([#20](https://github.com/nicoarbelaez/claude-identity/issues/20)) ([557f52b](https://github.com/nicoarbelaez/claude-identity/commit/557f52b04d5834882078dbef31e48461e1a1dc67))

## [0.2.0](https://github.com/nicoarbelaez/claude-identity/compare/v0.1.0...v0.2.0) (2026-10-26)


### Features

* CLI cid con list, switch, add, remove, current, config y version ([#12](https://github.com/nicoarbelaez/claude-identity/issues/12)) ([7ad7a62](https://github.com/nicoarbelaez/claude-identity/commit/7ad7a6250f40790bbbe646947da95e18198fe766))


### Bug Fixes

* **release:** usar tags vX.Y.Z y permitir reconstruir una release ([#11](https://github.com/nicoarbelaez/claude-identity/issues/11)) ([c7a20e5](https://github.com/nicoarbelaez/claude-identity/commit/c7a20e54f384d54fc623291fa2540169ab7b7ae4))

## 0.1.0 (2026-10-26)


### Features

* **i18n:** add en/es catalogs and language detection ([#6](https://github.com/nicoarbelaez/claude-identity/issues/6)) ([7ce1974](https://github.com/nicoarbelaez/claude-identity/commit/7ce1974e7c87e99f355e317206dfc04f0e6b1487))
* profile store and OS keyring vault (T2) ([#3](https://github.com/nicoarbelaez/claude-identity/issues/3)) ([d34ae6a](https://github.com/nicoarbelaez/claude-identity/commit/d34ae6a17ec3bfc772eb520cee0b71b9283caa97))
* swap engine (claudecfg stores + swap service) ([#7](https://github.com/nicoarbelaez/claude-identity/issues/7)) ([307f70b](https://github.com/nicoarbelaez/claude-identity/commit/307f70bd6412e868d5f9286bca6c3c0190be4d40))


### Bug Fixes

* **release:** fijar la versión inicial en 0.1.0 ([#10](https://github.com/nicoarbelaez/claude-identity/issues/10)) ([609339a](https://github.com/nicoarbelaez/claude-identity/commit/609339ad89f1092f9e23193dfbd252ec63ff6091))
