# caddy-fs-s3 (fork)

S3(및 호환 오브젝트 스토리지)를 Caddy 가상 파일시스템으로 마운트하는 모듈이다.
https://github.com/sagikazarmark/caddy-fs-s3 의 fork로 요청마다 커넥션이 새로 맺어지며 발생하는 성능 이슈를 해결했다.

## 무엇이 다른가

원본은 파일을 열 때 곧바로 `GetObject`로 본문 스트림을 연다. 그런데 Caddy의 `file_server`는 내부적으로 `http.ServeContent`를 쓰고, 이게 매 요청마다 파일 끝으로 seek해 크기를 잰 뒤 다시 처음으로 돌아온다. 이때 열어둔 본문을 **끝까지 읽지 않은 채 Close**하는데, HTTP/1.1에서는 본문을 다 안 읽고 닫은 커넥션이 keepalive 풀로 돌아가지 못하고 그냥 끊긴다. 그래서 keepalive가 켜져 있어도 **서빙마다 TCP+TLS 핸드셰이크를 새로 치르게 되어** 느려진다.

이 fork는 vendoring한 `s3fs`를 수정해 **본문을 첫 `Read` 시점에 지연 오픈한다.** seek 단계에서는 아직 열린 본문이 없어 커넥션을 버리지 않고, `GetObject`는 서빙당 한 번만 일어나며, 커넥션이 정상적으로 재사용된다.

| | 원본 | 이 fork |
|---|---|---|
| 서빙당 `GetObject` | 2회 | 1회 |
| 폐기되는 커넥션 | 매 요청 1개 | 없음 |

## 설치

[xcaddy](https://github.com/caddyserver/xcaddy)로 빌드한다.

```shell
xcaddy build --with github.com/dgdsingen/caddy-fs-s3
```

## 사용법

```caddyfile
{
	filesystem my-s3-fs s3 {
		bucket mybucket
		region us-east-1

		# endpoint <endpoint>
		# profile <profile>
		# use_path_style
	}
}

example.com {
	file_server {
		fs my-s3-fs
	}
}
```

인증은 AWS SDK 기본 자격 증명 체인을 따른다(환경변수, `~/.aws/credentials`, IAM 역할 등). 예:

```shell
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

## 설정 옵션

| 옵션 | 설명 |
|---|---|
| `bucket` | S3 버킷 이름 (필수) |
| `region` | 버킷 리전 |
| `profile` | 사용할 AWS 프로파일 |
| `endpoint` | 비표준 엔드포인트 (S3 호환 스토리지용) |
| `use_path_style` | path-style 주소 방식 사용 |
| `force_path_style` | (deprecated) `use_path_style` 사용 권장 |

## 크레딧

- 원본 모듈: [sagikazarmark/caddy-fs-s3](https://github.com/sagikazarmark/caddy-fs-s3)
- 파일시스템 구현: [jszwec/s3fs](https://github.com/jszwec/s3fs)
