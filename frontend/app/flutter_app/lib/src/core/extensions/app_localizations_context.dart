import 'package:flutter/widgets.dart';

import 'package:go_wind_wms/generated/l10n.dart';

extension LocalizedBuildContext on BuildContext {
  S get loc => S.of(this);
}
