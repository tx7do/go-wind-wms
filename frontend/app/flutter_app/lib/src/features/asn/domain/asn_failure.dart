/// ASN（到货通知单）领域失败类型。
///
/// 由 data 层 [AsnRepositoryImpl] 根据 [ApiException] 的分类映射产生。
sealed class AsnFailure implements Exception {
  const AsnFailure();
}

/// 凭证/会话问题（401 / 403-鉴权拒绝）。
final class AsnUnauthorizedFailure extends AsnFailure {
  const AsnUnauthorizedFailure();
}

/// 业务校验失败（409-CONFLICT / 400-BAD_REQUEST）：
/// 防超收、防超上架、质检闸门、状态机非法流转等。
final class AsnInvalidInputFailure extends AsnFailure {
  const AsnInvalidInputFailure();
}

/// 未找到（404）：单号不存在。
final class AsnNotFoundFailure extends AsnFailure {
  const AsnNotFoundFailure();
}

/// 网络或服务端错误（5xx / 连接失败 / 超时）。
final class AsnNetworkFailure extends AsnFailure {
  const AsnNetworkFailure();
}

/// 未分类错误。
final class AsnUnknownFailure extends AsnFailure {
  const AsnUnknownFailure();
}
