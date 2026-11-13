package com.github.kr328.clash

import android.content.Intent
import android.net.Uri
import android.os.Build
import java.util.Locale
import com.github.kr328.clash.design.R
import com.github.kr328.clash.design.TailnetDesign
import com.github.kr328.clash.service.store.ServiceStore
import com.github.kr328.clash.util.withClash
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.isActive
import kotlinx.coroutines.selects.select
import kotlinx.coroutines.selects.onTimeout
import org.json.JSONObject

class TailnetActivity : BaseActivity<TailnetDesign>() {
    override suspend fun main() {
        val store = ServiceStore(this)
        if (store.tailnetHostname in setOf("android-cmfa-validation", "android-cmfa", "cmfa-android")) {
            val model = Build.MODEL.lowercase(Locale.ROOT).replace(Regex("[^a-z0-9]+"), "-").trim('-').take(58).ifEmpty { "android" }
            store.tailnetHostname = "cmfa-$model"
        }
        val design = TailnetDesign(this, store, clashRunning)
        setContentDesign(design)

        suspend fun refresh(action: String) {
            if (!store.tailnetEnabled || !clashRunning) {
                design.showStatus(if (!store.tailnetEnabled) "disabled" else "stopped")
                return
            }
            try {
                val state = JSONObject(withClash { tailnetCommand(action) })
                val ips = state.optJSONArray("ips")
                val addresses = if (ips == null) "" else (0 until ips.length()).joinToString("\n") { ips.getString(it) }
                design.showStatus(state.optString("state"), addresses, state.optString("error"), state.optLong("keyExpiry"))
                if (action == "login") {
                    val url = state.optString("authUrl")
                    val uri = Uri.parse(url)
                    if (uri.scheme == "https" && uri.host == "login.tailscale.com" && uri.port == -1 && uri.path?.startsWith("/a/") == true) {
                        startActivity(Intent(Intent.ACTION_VIEW, uri))
                    } else if (url.isEmpty()) {
                        design.showStatus("unlogged", error = getString(R.string.tailnet_login_pending))
                    }
                }
            } catch (e: CancellationException) { throw e }
            catch (_: Exception) { design.showStatus("error", error = getString(R.string.tailnet_error)) }
        }
        refresh("status")
        while (isActive) {
            select<Unit> {
                events.onReceive {
                    when (it) {
                        Event.ClashStart, Event.ClashStop, Event.ServiceRecreated -> recreate()
                        else -> Unit
                    }
                }
                design.requests.onReceive {
                    when (it) {
                        TailnetDesign.Request.Changed -> refresh("status")
                        TailnetDesign.Request.Login -> refresh("login")
                        TailnetDesign.Request.Logout -> refresh("logout")
                        TailnetDesign.Request.Retry -> refresh("retry")
                    }
                }
                onTimeout(3000) { refresh("status") }
            }
        }
    }
}
