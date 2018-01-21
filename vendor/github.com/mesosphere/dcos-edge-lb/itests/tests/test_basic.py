import json
import logging
import re
import subprocess
import tempfile

import retrying
import yaml

# import pytest
# import test_helpers
# from dcos_test_utils import marathon
import app_types_helper
import edgelb
import util
import zero_downtime_helper

# EVEN IF A HELPER IMPLICITLY CALLS A CLI COMMAND ALREADY we should still
# have an explicit test for each command because the underlying implementation
# of the helper may change.

EDGELB = edgelb.EdgeLB()

util.configure_logger(logging.INFO)
logger = util.get_logger(__name__)


def setup_module():
    EDGELB.uninstall(ignore_error=True)
    edgelb.force_clean_pools(ignore_error=True)
    util.wipe_marathon_things()
    EDGELB.install()


def teardown_module():
    EDGELB.uninstall()
    edgelb.force_clean_pools(ignore_error=True)
    util.wipe_marathon_things()


def test_help():
    # Check is false because we expect a non-zero returncode

    o = util.sh("dcos edgelb", check=False, stdout=subprocess.PIPE)
    noarg_out = o.stdout
    assert o.returncode != 0
    assert len(o.stdout.decode().strip()) != 0
    lines = o.stdout.decode().splitlines()
    assert len(lines) > 1

    o = util.sh("dcos edgelb -h", check=False, stdout=subprocess.PIPE)
    dh_out = o.stdout
    assert o.returncode != 0
    assert len(o.stdout.decode().strip()) != 0
    lines = o.stdout.decode().splitlines()
    assert len(lines) > 1

    o = util.sh("dcos edgelb --help", check=False, stdout=subprocess.PIPE)
    ddh_out = o.stdout
    assert o.returncode != 0
    assert len(o.stdout.decode().strip()) != 0
    lines = o.stdout.decode().splitlines()
    assert len(lines) > 1

    # This command is actually a 0 return code. Should it be this way?
    o = util.sh("dcos edgelb help", stdout=subprocess.PIPE)
    help_out = o.stdout
    assert o.returncode == 0
    assert len(o.stdout.decode().strip()) != 0
    lines = o.stdout.decode().splitlines()
    assert len(lines) > 1

    assert noarg_out == dh_out
    assert noarg_out == ddh_out
    assert noarg_out == help_out


def test_ping():
    o = util.sh("dcos edgelb ping", stdout=subprocess.PIPE)
    assert o.stdout.decode() == "pong\n"


def test_version():
    o = util.sh("dcos edgelb version", stdout=subprocess.PIPE)
    edgelb_lines = o.stdout.decode().splitlines()
    assert len(edgelb_lines) == 2

    o = util.sh("dcos edgelb-pool version", stdout=subprocess.PIPE)
    edgelb_pool_lines = o.stdout.decode().splitlines()
    assert len(edgelb_pool_lines) == 1

    clientre = re.compile("^client = (v[0-9]+\.[0-9]+\.[0-9]+.*)$")
    serverre = re.compile("^server = (v[0-9]+\.[0-9]+\.[0-9]+.*)$")

    edgelb_client_m = clientre.match(edgelb_lines[0])
    edgelb_server_m = serverre.match(edgelb_lines[1])

    edgelb_pool_client_m = clientre.match(edgelb_pool_lines[0])

    assert edgelb_client_m is not None
    assert edgelb_server_m is not None
    assert edgelb_pool_client_m is not None

    edgelb_client_cap = edgelb_client_m.group(1)
    edgelb_server_cap = edgelb_server_m.group(1)
    edgelb_pool_client_cap = edgelb_pool_client_m.group(1)

    # Check that we captured nice strings
    assert edgelb_client_cap
    assert edgelb_server_cap
    assert edgelb_pool_client_cap

    # Check that they're all the same
    assert edgelb_client_cap == edgelb_server_cap
    assert edgelb_client_cap == edgelb_pool_client_cap


def test_config_reference():
    o = util.sh("dcos edgelb show --reference", stdout=subprocess.PIPE)
    assert len(o.stdout.decode().strip()) != 0
    lines = o.stdout.decode().splitlines()
    assert len(lines) > 800


def test_config():
    # Install pool using config
    poolname = "sample-minimal"
    with util.ExampleConfig("{}.yaml".format(poolname)) as c:
        util.retry_sh('dcos edgelb create {}'.format(c))

    edgelb.pool_up(poolname, 1)
    logger.info('pool {} is up'.format(poolname))

    edgelb.delete_all_pools(ignore_error=False)
    logger.info('pool {} deleted'.format(poolname))


def test_role_star():
    poolname = "sample-minimal"
    with util.ExampleConfig("{}.yaml".format(poolname)) as c_filename:
        y = None
        with open(c_filename, 'r') as c:
            y = yaml.load(c)
            y["pools"][0]["role"] = "*"
            # Decrease CPU for dcos-docker
            y["pools"][0]["cpus"] = 0.3
            # Change the bindPort because it may not be using public slave
            y["pools"][0]["haproxy"]["frontends"][0]["bindPort"] = 25808
        with open(c_filename, 'w') as c:
            # Wipe the file
            c.truncate(0)

            s = yaml.dump(y)
            logger.info(s)
            c.write(s)
            c.flush()
            util.retry_sh('dcos edgelb create {}'.format(c_filename))

    edgelb.pool_up(poolname, 1)

    # Delete pool with helper
    edgelb.delete_pool(poolname)


def test_lbconfig():
    poolname = "sample-minimal"

    with util.ExampleConfig("{}.yaml".format(poolname)) as c:
        util.sh('dcos edgelb create {}'.format(c))

    edgelb.pool_up(poolname, 1)

    # Get the raw haproxy.cfg from apiserver
    o = util.sh("dcos edgelb lb-config {} --raw".
                format(poolname), stdout=subprocess.PIPE)
    assert o.returncode == 0
    assert len(o.stdout.decode().strip()) != 0
    lraw = o.stdout.decode().strip().splitlines()
    assert len(lraw) > 1

    # Get the pretty haproxy.cfg from apiserver
    o = util.sh("dcos edgelb lb-config {}".
                format(poolname), stdout=subprocess.PIPE)
    assert o.returncode == 0
    assert len(o.stdout.decode().strip()) != 0
    l = o.stdout.decode().strip().splitlines()
    assert len(l) > 1
    assert len(l) < len(lraw)

    # Delete pool with helper
    edgelb.delete_pool(poolname)


def test_templates():
    poolname = "sample-minimal"
    artifactfile = "haproxy.cfg.ctmpl"

    def get_pool_artifact():
        o = util.sh("dcos edgelb template show {}".
                    format(poolname), stdout=subprocess.PIPE)
        assert o.returncode == 0
        assert len(o.stdout.decode().strip()) != 0
        l = o.stdout.decode().strip().splitlines()
        assert len(l) > 1
        return l

    with util.ExampleConfig("{}.yaml".format(poolname)) as c:
        util.sh('dcos edgelb create {}'.format(c))

    edgelb.pool_up(poolname, 1)

    # Get the default artifact from apiserver
    defaultlines = get_pool_artifact()

    # Get the default artifact from the repo
    with util.ExampleTemplate(artifactfile) as t_filename:
        with open(t_filename, 'r') as t:
            examplelines = t.readlines()
            assert len(examplelines) > 1
    assert(len(defaultlines) == len(examplelines))

    # Attempt to delete the default
    o = util.sh("dcos edgelb template delete {}".
                format(poolname), stdout=subprocess.PIPE)
    assert o.returncode == 0

    # Get the updated artifact from apiserver
    deletedlines = get_pool_artifact()
    assert(len(defaultlines) == len(deletedlines))

    # Copy the default, modify it, and upload it
    customlines = list(defaultlines)
    customlines.append("# Modification")
    with tempfile.NamedTemporaryFile(suffix=".cfg.ctmpl") as f:
        f.write("\n".join(customlines).encode())
        f.flush()
        o = util.sh("dcos edgelb template update {} {}".
                    format(poolname, f.name),
                    stdout=subprocess.PIPE)
        assert o.returncode == 0

    # Get the updated artifact from apiserver
    updatedlines = get_pool_artifact()
    assert(len(customlines) == len(updatedlines))
    assert(len(defaultlines) < len(updatedlines))

    # Delete the modified artifact (should result in reverting to default)
    o = util.sh("dcos edgelb template delete {}".
                format(poolname), stdout=subprocess.PIPE)
    assert o.returncode == 0

    # Get the updated artifact from apiserver
    updateddeletedlines = get_pool_artifact()
    assert(len(defaultlines) == len(updateddeletedlines))

    # Delete pool with helper
    edgelb.delete_pool(poolname)


def test_task_app_types():
    app_types_helper.task_app_types_tester()


def test_pod_app_types():
    app_types_helper.pod_app_types_tester()


def test_path_routing():
    class AppInfo:
        def __init__(self, prefix):
            self.prefix = prefix
            self.appid = "{}-svc".format(self.prefix)
            self.portname = "{}-port".format(self.prefix)
            self.ret_id = util.rand_str(5)

    def query_lb(lb_taskid, lb_host, path):
        fmts = ("dcos task exec {} bash -c 'wget -qO- http://{}{}'")
        o = util.retry_sh(fmts.format(lb_taskid, lb_host, path),
                          stdout=subprocess.PIPE)
        return o.stdout.decode().strip()

    @retrying.retry(wait_fixed=util.DEFAULT_WAIT * 1000,
                    stop_max_delay=util.DEFAULT_TIMEOUT * 1000,
                    retry_on_exception=lambda x: True)
    def query_wait(lb_taskid, lb_host, path, expectedFunc):
        got = query_lb(lb_taskid, lb_host, path)
        logger.info("got {}".format(got))
        assert expectedFunc(got)

    appinfos = [AppInfo(x) for x in ["foo", "bar", "baz", "default"]]
    for appinfo in appinfos:
        with util.ExampleApp("host-mesos-id.json") as a_filename:
            j = None
            with open(a_filename, 'r') as a:
                j = json.load(a)

                j["id"] = "/{}".format(appinfo.appid)
                j["portDefinitions"][0]["name"] = appinfo.portname
                j["cmd"] = "{} {}".format(j["cmd"], appinfo.ret_id)
            with open(a_filename, 'w') as a:
                # Wipe the file
                a.truncate(0)

                s = json.dumps(j)
                logger.info(s)
                a.write(s)
                a.flush()
                util.retry_sh("dcos marathon app add {}".format(a_filename))

    def ret_id_success(s, prefix):
        for appinfo in appinfos:
            if appinfo.prefix == prefix:
                return appinfo.ret_id == s
        return False

    def foo_success(s):
        return ret_id_success(s, "foo")

    def bar_success(s):
        return ret_id_success(s, "bar")

    def baz_success(s):
        return ret_id_success(s, "baz")

    def default_success(s):
        return ret_id_success(s, "default")

    poolname = "sample-path-routing"
    with util.ExampleConfig("{}.yaml".format(poolname)) as c:
        util.sh('dcos edgelb create {}'.format(c))

    edgelb.pool_up(poolname, 1)

    tids = edgelb.get_lb_tids(poolname)
    assert len(tids) == 1
    tid = tids[0]

    query_wait(tid, "localhost", "/foo", foo_success)
    query_wait(tid, "localhost", "/bar/id", bar_success)
    query_wait(tid, "localhost", "/baz/id", baz_success)
    query_wait(tid, "localhost", "/id", default_success)

    util.kill_marathon_apps([x.appid for x in appinfos])

    edgelb.delete_pool(poolname)


def test_zero_downtime_reload():
    zero_downtime_helper.zero_downtime_reload_tester()
