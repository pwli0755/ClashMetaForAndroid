package com.github.kr328.clash.service.clash.module

import android.app.Service
import com.github.kr328.clash.common.constants.Intents
import com.github.kr328.clash.common.log.Log
import com.github.kr328.clash.core.Clash
import com.github.kr328.clash.service.util.configureTailnet
import com.github.kr328.clash.service.util.RuntimeProfiles
import com.github.kr328.clash.service.StatusProvider
import com.github.kr328.clash.service.ProfileProcessor
import com.github.kr328.clash.service.data.ImportedDao
import com.github.kr328.clash.service.data.SelectionDao
import com.github.kr328.clash.service.store.ServiceStore
import com.github.kr328.clash.service.util.importedDir
import com.github.kr328.clash.service.util.sendProfileLoaded
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.selects.select
import java.util.*

class ConfigurationModule(service: Service) : Module<ConfigurationModule.LoadException>(service) {
    data class LoadException(val message: String)

    private val store = ServiceStore(service)
    private val runtimeProfiles = RuntimeProfiles(service.filesDir.resolve("runtime-profiles"))
    private val reload = Channel<Unit>(Channel.CONFLATED)

    override suspend fun run() {
        val broadcasts = receiveBroadcast {
            addAction(Intents.ACTION_PROFILE_CHANGED)
            addAction(Intents.ACTION_OVERRIDE_CHANGED)
        }

        var loaded: UUID? = null

        reload.trySend(Unit)

        while (true) {
            val changed: UUID? = select {
                broadcasts.onReceive {
                    if (it.action == Intents.ACTION_PROFILE_CHANGED)
                        UUID.fromString(it.getStringExtra(Intents.EXTRA_UUID))
                    else
                        null
                }
                reload.onReceive {
                    null
                }
            }

            try {
                val current = store.activeProfile
                    ?: throw NullPointerException("No profile selected")

                if (current == loaded && changed != null && changed != loaded)
                    continue

                service.configureTailnet()

                val active = ImportedDao().queryByUUID(current)
                    ?: throw NullPointerException("No profile selected")

                Clash.setAgeSecretKey(active.ageSecretKey?.takeIf { it.isNotBlank() })

                ProfileProcessor.withProfileFiles {
                    runtimeProfiles.activate(service.importedDir.resolve(active.uuid.toString()), active.uuid, active.name, active.ageSecretKey) {
                        Clash.load(it).await()
                    }
                }

                val remove = SelectionDao().querySelections(active.uuid)
                    .filterNot { Clash.patchSelector(it.proxy, it.selected) }
                    .map { it.proxy }

                SelectionDao().removeSelections(active.uuid, remove)

                loaded = current
                StatusProvider.currentProfile = active.name

                service.sendProfileLoaded(current)

                Log.d("Profile ${active.name} loaded")
            } catch (e: Exception) {
                if (loaded == null) {
                    val previous = runtimeProfiles.current()
                    if (previous == null) return enqueueEvent(LoadException(e.message ?: "Unknown"))
                    try {
                        Clash.setAgeSecretKey(previous.secret)
                        Clash.load(previous.directory).await()
                        loaded = previous.uuid
                        store.activeProfile = previous.uuid
                        StatusProvider.currentProfile = previous.name
                        service.sendProfileLoaded(previous.uuid)
                    } catch (fallback: Exception) {
                        return enqueueEvent(LoadException("${e.message}; recovery failed: ${fallback.message}"))
                    }
                } else if (store.activeProfile != loaded) {
                    // Keep selection and the running generation consistent after a rejected switch.
                    store.activeProfile = loaded
                    service.sendProfileLoaded(loaded)
                }
                Log.w("Configuration rejected; previous connection retained: ${e.message}")
            }
        }
    }
}
