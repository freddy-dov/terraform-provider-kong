package kong

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/WeKnowSports/terraform-provider-kong/helper"
	"github.com/dghubble/sling"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Plugin : Kong Service/API plugin request object structure
type Plugin struct {
	ID            string                 `json:"id,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Configuration map[string]interface{} `json:"config,omitempty"`
	Protocols     []string               `json:"protocols,omitempty"`
	Service       string                 `json:"-"`
	Route         string                 `json:"-"`
	Consumer      string                 `json:"-"`
	Tags          []string               `json:"tags"`
	Enabled       bool                   `json:"enabled"`
}

func resourceKongPlugin() *schema.Resource {
	return &schema.Resource{
		Create: resourceKongPluginCreate,
		Read:   resourceKongPluginRead,
		Update: resourceKongPluginUpdate,
		Delete: resourceKongPluginDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Default:     nil,
				Description: "The name of the plugin to use.",
			},

			"protocols": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    true,
				Default:     nil,
				Description: "A list of the request protocols that will trigger this plugin",
			},

			"config_json": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  nil,
			},

			"service": {
				Type:          schema.TypeString,
				Optional:      true,
				Default:       nil,
				ConflictsWith: []string{"route", "consumer"},
				Description:   "The id of the route to scope this plugin to. f set, the plugin will only activate when receiving requests via one of the routes belonging to the specified Service",
			},

			"route": {
				Type:          schema.TypeString,
				Optional:      true,
				Default:       nil,
				ConflictsWith: []string{"service", "consumer"},
				Description:   "The id of the route to scope this plugin to. If set, the plugin will only activate when receiving requests via the specified route",
			},

			"consumer": {
				Type:          schema.TypeString,
				Optional:      true,
				Default:       nil,
				ConflictsWith: []string{"service", "route"},
				Description:   "The id of the consumer to scope this plugin to. If set, the plugin will activate only for requests where the specified has been authenticated",
			},

			"tags": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				Description: "An optional set of strings associated with the Service for grouping and filtering.",
			},

			"enabled": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Whether the Service is active",
				Default:     true,
			},
		},
	}
}

func resourceKongPluginCreate(d *schema.ResourceData, meta interface{}) error {
	request := buildModifyRequest(d, meta)
	p := &Plugin{}

	if service, ok := d.GetOk("service"); ok {
		request = request.Path("services/").Path(service.(string) + "/")
	} else if route, ok := d.GetOk("route"); ok {
		request = request.Path("routes/").Path(route.(string) + "/")
	} else if consumer, ok := d.GetOk("consumer"); ok {
		request = request.Path("consumers/").Path(consumer.(string) + "/")
	}

	response, err := request.Post("plugins/").ReceiveSuccess(p)
	if err != nil {
		return fmt.Errorf("error while creating plugin: " + err.Error())
	}

	if response.StatusCode == http.StatusConflict {
		return fmt.Errorf("409 Conflict - use terraform import to manage this plugin")
	} else if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code received: " + response.Status)
	}

	return setPluginToResourceData(d, p)
}

func resourceKongPluginRead(d *schema.ResourceData, meta interface{}) error {
	sling := meta.(*sling.Sling)

	p := &Plugin{}

	response, err := sling.New().Path("plugins/").Get(d.Id()).ReceiveSuccess(p)
	if err != nil {
		return fmt.Errorf("error while updating plugin: " + err.Error())
	}

	if response.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code received: " + response.Status)
	}

	return setPluginToResourceData(d, p)
}

func resourceKongPluginUpdate(d *schema.ResourceData, meta interface{}) error {
	request := buildModifyRequest(d, meta)

	p := &Plugin{}

	response, err := request.Path("plugins/").Patch(d.Id()).ReceiveSuccess(p)
	if err != nil {
		return fmt.Errorf("error while updating plugin: " + err.Error())
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code received: " + response.Status)
	}

	return setPluginToResourceData(d, p)
}

func resourceKongPluginDelete(d *schema.ResourceData, meta interface{}) error {
	sling := meta.(*sling.Sling)

	response, error := sling.New().Path("plugins/").Delete(d.Id()).ReceiveSuccess(nil)
	if error != nil {
		return fmt.Errorf("error while deleting plugin: " + error.Error())
	}

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code received: " + response.Status)
	}

	return nil
}

func buildModifyRequest(d *schema.ResourceData, meta interface{}) *sling.Sling {
	request := meta.(*sling.Sling).New()

	plugin := &Plugin{
		ID:        d.Id(),
		Name:      d.Get("name").(string),
		Protocols: helper.ConvertInterfaceArrToStrings(d.Get("protocols").([]interface{})),
		Service:   d.Get("service").(string),
		Route:     d.Get("route").(string),
		Consumer:  d.Get("consumer").(string),
		Tags:      helper.ConvertInterfaceArrToStrings(d.Get("tags").([]interface{})),
		Enabled:   d.Get("enabled").(bool),
	}

	if c, ok := d.GetOk("config_json"); ok {
		config := make(map[string]interface{})
		err := json.Unmarshal([]byte(c.(string)), &config)
		if err != nil {
			fmt.Printf("JSON is invalid")
		}

		plugin.Configuration = config

		request = request.BodyJSON(plugin)
	} else {
		form := url.Values{
			"name": {plugin.Name},
		}

		body := strings.NewReader(form.Encode())

		request = request.Body(body).Set("Content-Type", "application/x-www-form-urlencoded")
	}

	return request
}

func setPluginToResourceData(d *schema.ResourceData, plugin *Plugin) error {
	d.SetId(plugin.ID)

	_ = d.Set("name", plugin.Name)

	// There are differences in the way service/route IDs are returned from Kong after creation and update between
	// version before and after 1.0.0. We are risking some drift here. This will be handled in later versions.
	if service, ok := d.GetOk("service"); ok {
		plugin.Service = service.(string)
	} else if route, ok := d.GetOk("route"); ok {
		plugin.Route = route.(string)
	} else if consumer, ok := d.GetOk("consumer"); ok {
		plugin.Consumer = consumer.(string)
	}

	_ = d.Set("protocols", plugin.Protocols)
	_ = d.Set("service", plugin.Service)
	_ = d.Set("route", plugin.Route)
	_ = d.Set("consumer", plugin.Consumer)
	_ = d.Set("tags", plugin.Tags)
	_ = d.Set("enabled", plugin.Enabled)

	return nil
}
