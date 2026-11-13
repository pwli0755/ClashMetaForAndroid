package com.github.kr328.clash.service.util

import android.content.Context
import com.github.kr328.clash.core.bridge.Bridge
import com.github.kr328.clash.service.model.AccessControlMode
import com.github.kr328.clash.service.store.ServiceStore
import org.json.JSONArray
import org.json.JSONObject

/** Shared UIDs get proxy permission only when every package sharing that UID is allowed. */
internal fun proxyUIDs(mode: AccessControlMode, selected: Set<String>, packages: Map<Int, List<String>>): List<Int> =
    packages.filter { (uid, names) ->
        uid >= 10000 && names.isNotEmpty() && names.all {
            when (mode) {
                AccessControlMode.AcceptAll -> true
                AccessControlMode.AcceptSelected -> it in selected
                AccessControlMode.DenySelected -> it !in selected
            }
        }
    }.keys.toList()

fun Context.configureTailnet() {
    val store = ServiceStore(this)
    check(!store.tailnetEnabled || android.os.Build.VERSION.SDK_INT >= 29) { "Tailscale integration requires Android 10 or newer" }
    val packages = packageManager.getInstalledPackages(0)
        .filter { it.applicationInfo != null }
        .groupBy { it.applicationInfo!!.uid }
        .mapValues { (_, entries) -> entries.map { it.packageName } }
    val data = JSONObject()
        .put("enabled", store.tailnetEnabled)
        .put("hostname", store.tailnetHostname)
        .put("acceptRoutes", store.tailnetAcceptRoutes)
        .put("allowedUids", JSONArray(proxyUIDs(store.accessControlMode, store.accessControlPackages, packages)))
    val error = Bridge.nativeConfigureTailnet(data.toString())
    check(error.isEmpty()) { error }
}

fun Context.tailnetProxyPermitted(uid: Int): Boolean {
    val store = ServiceStore(this)
    if (!store.tailnetEnabled) return true
    val names = packageManager.getPackagesForUid(uid)?.toList().orEmpty()
    return proxyUIDs(store.accessControlMode, store.accessControlPackages, mapOf(uid to names)).isNotEmpty()
}
