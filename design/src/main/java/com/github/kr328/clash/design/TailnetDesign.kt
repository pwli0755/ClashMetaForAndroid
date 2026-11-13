package com.github.kr328.clash.design

import android.content.Context
import android.os.Build
import android.view.View
import com.github.kr328.clash.design.databinding.DesignSettingsCommonBinding
import com.github.kr328.clash.design.databinding.ViewTailnetBinding
import com.github.kr328.clash.design.dialog.requestModelTextInput
import com.github.kr328.clash.design.util.*
import com.github.kr328.clash.service.store.ServiceStore
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import kotlinx.coroutines.launch
import java.text.DateFormat
import java.util.Date

class TailnetDesign(context: Context, private val store: ServiceStore, private val running: Boolean) : Design<TailnetDesign.Request>(context) {
    enum class Request { Login, Logout, Retry, Changed }
    private val binding = DesignSettingsCommonBinding.inflate(context.layoutInflater, context.root, false)
    private val content = ViewTailnetBinding.inflate(context.layoutInflater, binding.content, false)
    override val root: View get() = binding.root
    private var connectAction = Request.Login

    fun showStatus(state: String, addresses: String = "", error: String = "", expiry: Long = 0) {
        val label = when (state) {
            "connected" -> R.string.tailnet_connected
            "connecting" -> R.string.tailnet_connecting
            "unlogged" -> R.string.tailnet_unlogged
            "disabled" -> R.string.tailnet_disabled
            "stopped" -> R.string.tailnet_stopped
            "error" -> R.string.tailnet_status_error
            else -> R.string.tailnet_offline
        }
        content.statusTitle.setText(label)
        content.statusDetail.text = error.ifBlank { context.getString(when (state) {
            "connected" -> R.string.tailnet_connected_detail
            "disabled" -> R.string.tailnet_disabled_detail
            "stopped" -> R.string.tailnet_start
            "unlogged" -> R.string.tailnet_login_detail
            else -> R.string.tailnet_retry_detail
        }) }
        content.addresses.text = addresses
        content.addresses.visibility = if (addresses.isBlank()) View.GONE else View.VISIBLE
        content.expiry.visibility = if (addresses.isBlank()) View.GONE else View.VISIBLE
        content.expiry.text = if (expiry > 0) context.getString(R.string.tailnet_expiry, DateFormat.getDateInstance(DateFormat.MEDIUM).format(Date(expiry * 1000)))
            else context.getString(R.string.tailnet_expiry_managed)
        content.connect.visibility = if (state == "unlogged" || state == "offline" || state == "error") View.VISIBLE else View.GONE
        connectAction = if (state == "unlogged") Request.Login else Request.Retry
        content.connect.setText(if (connectAction == Request.Login) R.string.tailnet_login else R.string.tailnet_retry)
        content.logout.visibility = if (running && addresses.isNotBlank()) View.VISIBLE else View.GONE
    }

    suspend fun rename() {
        val name = context.requestModelTextInput(
            initial = store.tailnetHostname,
            title = context.getString(R.string.tailnet_hostname),
            hint = context.getString(R.string.tailnet_hostname_hint),
            error = context.getString(R.string.tailnet_hostname_invalid),
            validator = { it.matches(Regex("[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?")) }
        )
        if (name != store.tailnetHostname) {
            store.tailnetHostname = name
            content.hostname.text = name
            requests.trySend(Request.Changed)
        }
    }

    init {
        binding.surface = surface
        binding.activityBarLayout.applyFrom(context)
        binding.scrollRoot.bindAppBarElevation(binding.activityBarLayout)
        binding.content.addView(content.root)
        content.hostname.text = store.tailnetHostname
        content.enabledSwitch.isChecked = store.tailnetEnabled
        content.routesSwitch.isChecked = store.tailnetAcceptRoutes
        content.enabledSwitch.isEnabled = !running && Build.VERSION.SDK_INT >= 29
        content.routesSwitch.isEnabled = !running
        content.lockedHint.visibility = if (running) View.VISIBLE else View.GONE
        content.enabledSwitch.setOnCheckedChangeListener { _, checked ->
            store.tailnetEnabled = checked
            requests.trySend(Request.Changed)
        }
        content.routesSwitch.setOnCheckedChangeListener { _, checked -> store.tailnetAcceptRoutes = checked }
        content.hostnameRow.setOnClickListener { launch { rename() } }
        content.connect.setOnClickListener { requests.trySend(connectAction) }
        content.help.setOnClickListener {
            MaterialAlertDialogBuilder(context).setTitle(R.string.tailnet_help)
                .setMessage(R.string.tailnet_help_detail).setPositiveButton(R.string.ok, null).show()
        }
        content.logout.setOnClickListener {
            MaterialAlertDialogBuilder(context).setTitle(R.string.tailnet_logout)
                .setMessage(R.string.tailnet_logout_confirm)
                .setNegativeButton(R.string.cancel, null)
                .setPositiveButton(R.string.tailnet_logout) { _, _ -> requests.trySend(Request.Logout) }.show()
        }
        showStatus(if (!store.tailnetEnabled) "disabled" else if (!running) "stopped" else "connecting")
    }
}
