// LX04 EFS DNS retry loop: independent, bounded one-shot recovery.
// Proven in one OEM APK on one device. NOT a universal Xiaomi patch.
// Never hook all HTTP or suppress logs globally.
// Temporary Java connect() hooks are removed before this script detaches.
'use strict';
var count = 0;
var hooks = [];
function note(s) { send(['frida:stdout', 'XIAONEXUS_EFS_' + s + '\n']); }
Java.perform(function () {
  try {
    var Config = Java.use('com.efs.sdk.base.a.c.a.c');
    var Center = Java.use('com.efs.sdk.base.a.d.a');
    var Env = Java.use('com.efs.sdk.base.a.c.a');
    var ioException = Java.use('java.io.IOException');
    var Thread = Java.use('java.lang.Thread');
    function fld(k, name) {
      var f = k.class.getDeclaredField(name);
      f.setAccessible(true);
      return f;
    }
    var config = Config.a.overload().call(Config);
    var env = fld(Center, 'a').get(null);
    if (env === null) throw Error('EFS GlobalEnv absent');
    var refresh = fld(Config, 'c');
    var upload = fld(Env, 'e');
    var stat = fld(Env, 'd');
    refresh.setBoolean(config, false);
    upload.setBoolean(env, false);
    stat.setBoolean(env, false);
    if (refresh.getBoolean(config) || upload.getBoolean(env) || stat.getBoolean(env)) {
      throw Error('EFS flags did not become false');
    }
    note('FLAGS_FALSE');

    var targets = [
      'com.android.okhttp.internal.huc.HttpURLConnectionImpl',
      'com.android.okhttp.internal.huc.HttpsURLConnectionImpl'
    ];
    targets.forEach(function (name) {
      try {
        var K = Java.use(name);
        var connect = K.connect.overload();
        connect.implementation = function () {
          var isEfs = false;
          try {
            if (String(Thread.currentThread().getName()) === 'efs-base') {
              var path = String(this.getURL().getPath());
              isEfs = path === '/apm_cc';
            }
          } catch (_) { isEfs = false; }
          if (isEfs && count < 9) {
            count++;
            note('ABORT_' + count);
            // IOException is caught by the original DEX's generic Throwable handler.
            // Unlike UnknownHostException, that handler increments the loop counter.
            throw ioException.$new('XiaoNexus EFS one-shot loop exit');
          }
          return connect.call(this);
        };
        hooks.push(connect);
        note('HOOK_READY_' + hooks.length);
      } catch (e) { note('CLASS_SKIP'); }
    });
    if (hooks.length === 0) {
      note('NO_CONNECT_HOOK');
      return;
    }
    note('READY');
    setTimeout(function () {
      hooks.forEach(function (c) { try { c.implementation = null; } catch (_) {} });
      note('RESULT_HITS_' + count + '_HOOKS_' + hooks.length);
    }, 7000);
  } catch (ex) {
    hooks.forEach(function (c) { try { c.implementation = null; } catch (_) {} });
    note('ERROR_' + String(ex).replace(/[^A-Za-z0-9 _.-]/g, '_').slice(0, 100));
  }
});
