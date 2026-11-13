package com.github.kr328.clash.service.util

import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.File
import java.io.IOException
import java.util.UUID

/** Runtime generations never share mutable files with a subscription download. */
internal class RuntimeProfiles(private val root: File) {
    data class Saved(val directory: File, val uuid: UUID, val name: String, val secret: String?)

    fun current(): Saved? = runCatching {
        DataInputStream(root.resolve("current").inputStream()).use {
            val generation = UUID.fromString(it.readUTF()).toString()
            Saved(root.resolve(generation), UUID.fromString(it.readUTF()), it.readUTF(), it.readUTF().ifEmpty { null })
        }
    }.getOrNull()

    suspend fun activate(source: File, uuid: UUID, name: String, secret: String?, load: suspend (File) -> Unit): Saved {
        check(root.isDirectory || root.mkdirs()) { "Cannot create runtime directory" }
        val previous = current()
        val generation = UUID.randomUUID().toString()
        val directory = root.resolve(generation)
        val marker = root.resolve("current.candidate")
        var applied = false
        try {
            check(source.copyRecursively(directory)) { "Cannot stage runtime profile" }
            DataOutputStream(marker.outputStream()).use {
                it.writeUTF(generation); it.writeUTF(uuid.toString()); it.writeUTF(name); it.writeUTF(secret.orEmpty())
            }
            load(directory)
            applied = true
            if (!marker.renameTo(root.resolve("current"))) throw IOException("Cannot retain last working runtime profile")
        } catch (e: Exception) {
            // A validated generation may already be serving traffic if marker commit failed.
            // Retain its files as well as the previous committed generation.
            marker.delete()
            if (!applied) directory.deleteRecursively()
            throw e
        }
        root.listFiles()?.filter { it.isDirectory && it != directory && it != previous?.directory }?.forEach { it.deleteRecursively() }
        return Saved(directory, uuid, name, secret)
    }
}
