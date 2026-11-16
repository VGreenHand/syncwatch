# 保留占位：当前 minifyEnabled = false，此文件暂未生效。
# 若将来开启 minifyEnabled，请补充以下规则（否则同步协议里的反射/序列化字段会被混淆）：
#   -keep class com.syncwatch.app.Protocol$* { *; }
#   -keepclassmembers class com.syncwatch.app.** { @androidx.annotation.Keep <fields>; }
#   -dontwarn org.conscrypt.**
